package wire

import (
	"cognida/internal/handler"
	l "cognida/internal/model/lifecycle"
	t "cognida/internal/model/taskcontrol"
	"cognida/internal/repository/mysql"
	"cognida/internal/service/knowledge"
	"cognida/internal/service/lifecycle"
	"cognida/internal/service/strategy"
	tc "cognida/internal/service/taskcontrol"
	"context"
	"gorm.io/gorm"
)

func (f *FeatureServices) configureStrategies(db *gorm.DB, auth lifecycle.Authorizer, compute lifecycle.Compute) {
	f.Strategies = strategy.NewService(mysql.NewStrategyRepository(db), f.Lifecycle, strategy.Authorizer(auth), map[string]strategy.Consumer{
		"cleaning":      {Name: "lifecycle.cleaning.v1", Baseline: lifecycle.DefaultCleaningStrategy},
		"retrieval":     {Name: "knowledge.retrieval.v1", Baseline: knowledge.DefaultRetrievalStrategy},
		"experience":    {Name: "agent.guidance.v1", Baseline: strategy.DefaultGuidance},
		"skill":         {Name: "agent.guidance.v1", Baseline: strategy.DefaultGuidance},
		"playbook":      {Name: "agent.guidance.v1", Baseline: strategy.DefaultGuidance},
		"orchestration": {Name: "taskcontrol.concurrency.v1", Baseline: strategy.DefaultOrchestrationStrategy},
	})
	f.Lifecycle.WithStrategies(f.Strategies)
	f.EvaluationSnapshots.WithStrategies(f.Strategies, compute)
	f.Strategies.WithOrchestration(f.TaskControl.OrchestrationTrialBinding, f.TaskControl.OrchestrationRegressionReader(f.Strategies))
	f.TaskRuntime.WithOrchestration(func(ctx context.Context, p t.Scope, id string, plan tc.PlanSpec) (int, error) {
		limit, _, e := f.Strategies.ResolveOrchestration(ctx, l.Principal{TenantID: p.TenantID, UserID: p.UserID}, id, tc.OrchestrationPlanDigest(plan), plan.Budget.MaxConcurrency)
		return limit, e
	})
	f.StrategyHandler = handler.NewStrategyHandler(f.Strategies)
}
