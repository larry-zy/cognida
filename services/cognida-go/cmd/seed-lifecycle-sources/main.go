// seed-lifecycle-sources prepares actual upstream data for a local browser run.
// It only accepts an isolated cognida_review_* database and loopback API/Redis.
// Created resources are retained for browser verification, with handles in the
// output manifest; drop the isolated review database during environment cleanup.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	agentctx "cognida/internal/model/agent"
	"cognida/internal/repository/mysql"
	"cognida/internal/service/agent/resultstore"
	"cognida/internal/service/agent/tools"
	"cognida/internal/service/datasource"

	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type fixture struct {
	TenantID          int64             `json:"tenant_id"`
	UserID            int64             `json:"user_id"`
	Schema            string            `json:"schema"`
	Table             string            `json:"table"`
	DatasourceID      string            `json:"datasource_id"`
	SessionID         string            `json:"session_id"`
	ResultID          string            `json:"result_id"`
	ResultIDs         []string          `json:"result_ids,omitempty"`
	ResultExpiresAt   time.Time         `json:"result_expires_at"`
	ResultRows        int               `json:"result_rows"`
	ResultColumnTypes map[string]string `json:"result_column_types,omitempty"`
	DatasetID         string            `json:"dataset_id"`
	EvaluationTaskID  string            `json:"evaluation_task_id"`
	Evaluation        map[string]any    `json:"evaluation"`
	Upstream          map[string]string `json:"upstream"`
}

type apiClient struct {
	base, token string
	client      *http.Client
}

func (a apiClient) call(method, path string, input, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(method, a.base+"/api/v1"+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("Idempotency-Key", uuid.NewString())
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("local API unavailable for %s", path)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		if method == http.MethodDelete && resp.StatusCode == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("%s %s returned HTTP %d", method, path, resp.StatusCode)
	}
	if output == nil {
		return nil
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return err
	}
	return json.Unmarshal(envelope.Data, output)
}
func save(path string, f *fixture) error {
	body, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0600)
}
func run(dir string, statusOnly, cleanup bool) error {
	cfg, err := driver.ParseDSN(os.Getenv("MYSQL_DSN"))
	if err != nil || !strings.HasPrefix(cfg.DBName, "cognida_review_") {
		return fmt.Errorf("MYSQL_DSN must select an isolated cognida_review_* database")
	}
	if os.Getenv("ISOLATED") != "true" {
		return fmt.Errorf("ISOLATED=true must explicitly confirm this disposable environment")
	}
	base := strings.TrimRight(os.Getenv("GO_SERVICE_URL"), "/")
	u, err := url.Parse(base)
	if err != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
		return fmt.Errorf("GO_SERVICE_URL must use loopback")
	}
	rhost, _, err := net.SplitHostPort(os.Getenv("REDIS_ADDR"))
	if err != nil || (rhost != "127.0.0.1" && rhost != "localhost") {
		return fmt.Errorf("REDIS_ADDR must use loopback")
	}
	var account struct {
		Auth struct {
			Data struct {
				AccessToken string `json:"access_token"`
				TenantID    int64  `json:"tenant_id"`
				User        struct {
					ID int64 `json:"id"`
				} `json:"user"`
			} `json:"data"`
		} `json:"auth"`
	}
	body, err := os.ReadFile(filepath.Join(dir, "account.json"))
	if err != nil {
		return fmt.Errorf("read account file: %w", err)
	}
	if err = json.Unmarshal(body, &account); err != nil {
		return err
	}
	if account.Auth.Data.TenantID != 1 || account.Auth.Data.User.ID != 1 || account.Auth.Data.AccessToken == "" {
		return fmt.Errorf("fixture account must be tenant 1 user 1")
	}
	api := apiClient{base: base, token: account.Auth.Data.AccessToken, client: &http.Client{Timeout: 45 * time.Second}}
	output := filepath.Join(dir, "source-fixtures.json")
	f := fixture{TenantID: 1, UserID: 1, Schema: cfg.DBName, Upstream: map[string]string{}}
	if old, err := os.ReadFile(output); err == nil {
		if err = json.Unmarshal(old, &f); err != nil {
			return err
		}
		if f.Schema != cfg.DBName || f.TenantID != 1 || f.UserID != 1 {
			return fmt.Errorf("existing manifest belongs to a different environment")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if f.Upstream == nil {
		f.Upstream = map[string]string{}
	}
	if cleanup {
		return cleanupFixture(api, cfg, output, &f)
	}
	if statusOnly {
		if f.EvaluationTaskID == "" {
			return fmt.Errorf("no evaluation task yet")
		}
		if err = api.call(http.MethodGet, "/evaluation/tasks/"+f.EvaluationTaskID, nil, &f.Evaluation); err != nil {
			return err
		}
		return save(output, &f)
	}
	db, err := gorm.Open(gormmysql.Open(os.Getenv("MYSQL_DSN")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return fmt.Errorf("cannot open isolated database")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	if f.Table == "" {
		f.Table = "browser_source_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if err = db.Exec("CREATE TABLE `" + f.Table + "` (id BIGINT PRIMARY KEY, region VARCHAR(32) NOT NULL, amount DECIMAL(12,2) NOT NULL, account_code VARCHAR(32) NOT NULL DEFAULT '00123', precise_amount DECIMAL(38,18) NOT NULL DEFAULT 9007199254740993.123456789012345678)").Error; err != nil {
			return err
		}
		if err = db.Exec("INSERT INTO `"+f.Table+"` (id,region,amount) VALUES (?,?,?),(?,?,?),(?,?,?)", int64(9007199254740993), "north", "12.50", int64(9007199254740994), "south", "7.25", int64(9007199254740995), "north", "3.00").Error; err != nil {
			return err
		}
		if err = save(output, &f); err != nil {
			return err
		}
	}
	if !regexp.MustCompile(`^browser_source_[a-f0-9]{32}$`).MatchString(f.Table) {
		return fmt.Errorf("invalid fixture table")
	}
	// Upgrade only this manifest's disposable source fixture, never business tables.
	for _, column := range []struct{ name, definition string }{
		{"account_code", "VARCHAR(32) NOT NULL DEFAULT '00123'"},
		{"precise_amount", "DECIMAL(38,18) NOT NULL DEFAULT 9007199254740993.123456789012345678"},
	} {
		var count int64
		if err = db.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = ? AND table_name = ? AND column_name = ?", cfg.DBName, f.Table, column.name).Scan(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			if err = db.Exec("ALTER TABLE `" + f.Table + "` ADD COLUMN `" + column.name + "` " + column.definition).Error; err != nil {
				return err
			}
		}
	}
	if err = db.Exec("UPDATE `"+f.Table+"` SET account_code = ?, precise_amount = ?", "00123", "9007199254740993.123456789012345678").Error; err != nil {
		return err
	}
	if f.DatasourceID == "" {
		host, portString, err := net.SplitHostPort(cfg.Addr)
		if err != nil {
			return err
		}
		port, err := strconv.Atoi(portString)
		if err != nil {
			return err
		}
		var ds struct {
			ID string `json:"id"`
		}
		if err = api.call(http.MethodPost, "/datasources", map[string]any{"name": f.Table, "description": "Isolated browser lifecycle source fixture", "type": "mysql", "host": host, "port": port, "database_name": cfg.DBName, "username": cfg.User, "password": cfg.Passwd}, &ds); err != nil {
			return err
		}
		if ds.ID == "" {
			return fmt.Errorf("datasource API returned no ID")
		}
		f.DatasourceID = ds.ID
		f.Upstream["datasource"] = "POST /api/v1/datasources; real isolated MySQL table with 3 rows"
		if err = save(output, &f); err != nil {
			return err
		}
	}
	if f.SessionID == "" {
		var session struct {
			ID       string `json:"id"`
			TenantID int64  `json:"tenant_id"`
			UserID   int64  `json:"user_id"`
		}
		if err = api.call(http.MethodPost, "/sessions", map[string]any{"title": "Browser source SQL fixture", "agent_type": "text2sql"}, &session); err != nil {
			return err
		}
		if session.ID == "" || session.TenantID != 1 || session.UserID != 1 {
			return fmt.Errorf("unexpected session owner")
		}
		f.SessionID = session.ID
		if err = save(output, &f); err != nil {
			return err
		}
	}
	cipher, err := datasource.NewCipher(os.Getenv("DATASOURCE_SECRET_KEY"))
	if err != nil {
		return err
	}
	manager := datasource.NewConnectionManager(mysql.NewDataSourceRepository(db), cipher, datasource.DefaultPoolOptions())
	defer manager.Close()
	redisDB, _ := strconv.Atoi(os.Getenv("REDIS_DB"))
	client := redis.NewClient(&redis.Options{Addr: os.Getenv("REDIS_ADDR"), Password: os.Getenv("REDIS_PASSWORD"), DB: redisDB})
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ctx = agentctx.WithSessionID(agentctx.WithUserID(agentctx.WithTenantID(ctx, 1), 1), f.SessionID)
	ctx = agentctx.WithToolScope(agentctx.WithDatasourceID(ctx, f.DatasourceID), "read")
	store := resultstore.NewRedisStore(client)
	tool := tools.NewSQLExecuteTool(db, manager, store, nil, resultstore.NewSessionRegistry(resultstore.DefaultTTL))
	request, _ := json.Marshal(tools.SQLExecuteRequest{SQL: "SELECT region, SUM(amount) AS revenue, COUNT(*) AS order_count, MIN(account_code) AS account_code, MAX(precise_amount) AS precise_amount FROM `" + f.Table + "` GROUP BY region ORDER BY region", DatabaseID: f.DatasourceID, MaxRows: 10})
	resultJSON, err := tool.InvokableRun(ctx, string(request))
	if err != nil {
		return err
	}
	var result tools.SQLExecuteResult
	if err = json.Unmarshal([]byte(resultJSON), &result); err != nil {
		return err
	}
	if result.ResultID == "" || result.RowCount != 2 {
		return fmt.Errorf("SQLTool did not produce expected 2-row stored result")
	}
	actual, err := store.GetExact(ctx, resultstore.OwnerKey(1, f.SessionID), result.ResultID)
	if err != nil {
		return err
	}
	if len(actual.Rows) != 2 || fmt.Sprint(actual.Rows[0]["revenue"]) != "15.50" || fmt.Sprint(actual.Rows[1]["revenue"]) != "7.25" {
		return fmt.Errorf("actual SQL aggregates did not match source rows")
	}
	if actual.ColumnTypes["revenue"] != "DECIMAL" || actual.ColumnTypes["region"] != "VARCHAR" {
		return fmt.Errorf("actual SQL result is missing driver column type provenance")
	}
	for _, row := range actual.Rows {
		if row["account_code"] != "00123" || row["precise_amount"] != "9007199254740993.123456789012345678" {
			return fmt.Errorf("actual SQL result changed exact fixture text")
		}
	}
	if len(f.ResultIDs) == 0 && f.ResultID != "" {
		f.ResultIDs = append(f.ResultIDs, f.ResultID)
	}
	f.ResultID, f.ResultRows, f.ResultExpiresAt = result.ResultID, result.RowCount, time.Now().Add(resultstore.DefaultTTL)
	f.ResultColumnTypes = actual.ColumnTypes
	f.ResultIDs = append(f.ResultIDs, result.ResultID)
	f.Upstream["result"] = "POST /api/v1/sessions -> real sql_execute tool SELECT/GROUP BY -> Redis ResultStore; refresh by rerunning this command"
	if err = save(output, &f); err != nil {
		return err
	}
	if f.DatasetID == "" {
		f.DatasetID = "browser_source_" + uuid.NewString()
		var created map[string]any
		if err = api.call(http.MethodPost, "/evaluation/datasets", map[string]any{"dataset_id": f.DatasetID, "name": "Browser deterministic graders", "evaluation_type": "qa", "qa_pairs": []map[string]string{{"question": "Reply with exactly the word blue.", "reference_answer": "blue"}, {"question": "What is 2 + 2? Reply with only the digit.", "reference_answer": "4"}}}, &created); err != nil {
			return err
		}
		if err = save(output, &f); err != nil {
			return err
		}
	}
	if f.EvaluationTaskID == "" {
		var task struct {
			ID string `json:"task_id"`
		}
		if err = api.call(http.MethodPost, "/evaluation/tasks", map[string]any{"dataset_id": f.DatasetID, "type": "qa", "config": map[string]any{"graders": []string{"rouge", "bleu"}}}, &task); err != nil {
			return err
		}
		if task.ID == "" {
			return fmt.Errorf("evaluation API returned no task ID")
		}
		f.EvaluationTaskID = task.ID
		f.Upstream["evaluation"] = "POST dataset with 2 QA samples -> POST evaluation task -> actual QA executor + configured model -> Python deterministic rouge/bleu graders; task status is read from API, never forged"
		if err = save(output, &f); err != nil {
			return err
		}
	}
	if err = api.call(http.MethodGet, "/evaluation/tasks/"+f.EvaluationTaskID, nil, &f.Evaluation); err != nil {
		return err
	}
	return save(output, &f)
}

func cleanupFixture(api apiClient, cfg *driver.Config, output string, f *fixture) error {
	if f.Table == "" || f.Schema != cfg.DBName || !regexp.MustCompile(`^browser_source_[a-f0-9]{32}$`).MatchString(f.Table) {
		return fmt.Errorf("cleanup requires a valid manifest for this isolated database")
	}
	for _, resource := range []struct{ path, id string }{{"/evaluation/tasks/", f.EvaluationTaskID}, {"/evaluation/datasets/", f.DatasetID}, {"/sessions/", f.SessionID}, {"/datasources/", f.DatasourceID}} {
		if resource.id != "" {
			if err := api.call(http.MethodDelete, resource.path+url.PathEscape(resource.id), nil, nil); err != nil {
				return err
			}
		}
	}
	redisDB, _ := strconv.Atoi(os.Getenv("REDIS_DB"))
	client := redis.NewClient(&redis.Options{Addr: os.Getenv("REDIS_ADDR"), Password: os.Getenv("REDIS_PASSWORD"), DB: redisDB})
	defer client.Close()
	ids := append(append([]string{}, f.ResultIDs...), f.ResultID)
	for _, id := range ids {
		if id != "" {
			if !strings.HasPrefix(id, "rs_") {
				return fmt.Errorf("invalid result handle in manifest")
			}
			if err := client.Del(context.Background(), "agent:resultstore:"+id).Err(); err != nil {
				return err
			}
		}
	}
	db, err := gorm.Open(gormmysql.Open(os.Getenv("MYSQL_DSN")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return fmt.Errorf("cannot open isolated database for cleanup")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	if err = db.Exec("DROP TABLE IF EXISTS `" + f.Table + "`").Error; err != nil {
		return err
	}
	return os.Rename(output, output+".cleaned")
}

func main() {
	dir := flag.String("dir", "", "isolated run directory containing account.json; output is source-fixtures.json")
	statusOnly := flag.Bool("status-only", false, "refresh the actual evaluation status without creating resources")
	cleanup := flag.Bool("cleanup", false, "delete only the upstream resources tracked in the manifest")
	flag.Parse()
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "-dir is required")
		os.Exit(1)
	}
	if *cleanup && *statusOnly {
		fmt.Fprintln(os.Stderr, "choose either -cleanup or -status-only")
		os.Exit(1)
	}
	if err := run(*dir, *statusOnly, *cleanup); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *cleanup {
		fmt.Println("Tracked fixture resources cleaned.")
	} else {
		fmt.Println("Source fixture handles saved; no credentials were written to the manifest.")
	}
}
