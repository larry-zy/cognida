// Package evaluation 数据集服务测试
package evaluation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	domeval "cognida/internal/model/evaluation"
)

// ========================================
// Mock DatasetRepository for DatasetManager Testing
// ========================================

type datasetManagerMockRepo struct {
	datasets  map[string]*domeval.Dataset     // datasetID -> dataset
	records   map[string][]*domeval.DatasetRecord // datasetID -> records
	nextID    int64
	createErr error
	findErr   error
	updateErr error
	deleteErr error
}

func newDatasetManagerMockRepo() *datasetManagerMockRepo {
	return &datasetManagerMockRepo{
		datasets: make(map[string]*domeval.Dataset),
		records:  make(map[string][]*domeval.DatasetRecord),
		nextID:   1,
	}
}

func (m *datasetManagerMockRepo) Create(ctx context.Context, dataset *domeval.Dataset) error {
	if m.createErr != nil {
		return m.createErr
	}
	dataset.ID = m.nextID
	m.nextID++
	m.datasets[dataset.DatasetID] = dataset
	return nil
}

func (m *datasetManagerMockRepo) FindByID(ctx context.Context, id string) (*domeval.Dataset, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	ds, ok := m.datasets[id]
	if !ok || ds.DeletedAt != nil {
		return nil, domeval.ErrDatasetNotFound
	}
	return ds, nil
}

func (m *datasetManagerMockRepo) FindByIDWithTenant(ctx context.Context, id string, tenantID int64) (*domeval.Dataset, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	ds, ok := m.datasets[id]
	if !ok || ds.DeletedAt != nil {
		return nil, domeval.ErrDatasetNotFound
	}
	if ds.TenantID != tenantID {
		return nil, domeval.ErrDatasetNotFound
	}
	return ds, nil
}

func (m *datasetManagerMockRepo) List(ctx context.Context, filter *domeval.DatasetFilter) ([]*domeval.Dataset, int64, error) {
	var result []*domeval.Dataset
	for _, ds := range m.datasets {
		if ds.DeletedAt != nil {
			continue
		}
		if filter.TenantID != nil && ds.TenantID != *filter.TenantID {
			continue
		}
		if filter.Type != nil && ds.Type != *filter.Type {
			continue
		}
		if filter.EvaluationType != nil && ds.EvaluationType != *filter.EvaluationType {
			continue
		}
		if filter.Search != "" && ds.Name != filter.Search {
			continue
		}
		result = append(result, ds)
	}
	total := int64(len(result))
	start := (filter.Page - 1) * filter.PageSize
	end := start + filter.PageSize
	if start >= len(result) {
		return []*domeval.Dataset{}, total, nil
	}
	if end > len(result) {
		end = len(result)
	}
	return result[start:end], total, nil
}

func (m *datasetManagerMockRepo) Update(ctx context.Context, dataset *domeval.Dataset) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	ds, ok := m.datasets[dataset.DatasetID]
	if !ok || ds.DeletedAt != nil {
		return domeval.ErrDatasetNotFound
	}
	ds.Name = dataset.Name
	ds.Description = dataset.Description
	ds.Type = dataset.Type
	ds.EvaluationType = dataset.EvaluationType
	return nil
}

func (m *datasetManagerMockRepo) SoftDelete(ctx context.Context, id string, tenantID int64) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	ds, ok := m.datasets[id]
	if !ok || ds.DeletedAt != nil {
		return domeval.ErrDatasetNotFound
	}
	if ds.TenantID != tenantID {
		return domeval.ErrDatasetNotFound
	}
	now := time.Now()
	ds.DeletedAt = &now
	return nil
}

func (m *datasetManagerMockRepo) AddSamples(ctx context.Context, datasetID string, samples []*domeval.DatasetRecord) error {
	if _, exists := m.datasets[datasetID]; !exists {
		return domeval.ErrDatasetNotFound
	}
	if _, exists := m.records[datasetID]; !exists {
		m.records[datasetID] = []*domeval.DatasetRecord{}
	}
	for _, r := range samples {
		r.ID = m.nextID
		m.nextID++
		m.records[datasetID] = append(m.records[datasetID], r)
	}
	if ds, ok := m.datasets[datasetID]; ok {
		ds.QACount += len(samples)
	}
	return nil
}

func (m *datasetManagerMockRepo) ListSamples(ctx context.Context, datasetID string, tenantID int64, page, pageSize int) ([]*domeval.DatasetRecord, int64, error) {
	records, exists := m.records[datasetID]
	if !exists {
		return []*domeval.DatasetRecord{}, 0, nil
	}
	var filtered []*domeval.DatasetRecord
	for _, r := range records {
		if r.TenantID == tenantID {
			filtered = append(filtered, r)
		}
	}
	total := int64(len(filtered))
	start := (page - 1) * pageSize
	end := start + pageSize
	if start >= len(filtered) {
		return []*domeval.DatasetRecord{}, total, nil
	}
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], total, nil
}

func (m *datasetManagerMockRepo) DeleteSample(ctx context.Context, datasetID string, tenantID int64, sampleID int64) error {
	records, exists := m.records[datasetID]
	if !exists {
		return domeval.ErrDatasetSampleNotFound
	}
	for i, r := range records {
		if r.ID == sampleID {
			if r.TenantID != tenantID {
				return domeval.ErrDatasetSampleNotFound
			}
			m.records[datasetID] = append(records[:i], records[i+1:]...)
			if ds, ok := m.datasets[datasetID]; ok && ds.QACount > 0 {
				ds.QACount--
			}
			return nil
		}
	}
	return domeval.ErrDatasetSampleNotFound
}

func (m *datasetManagerMockRepo) CountSamples(ctx context.Context, datasetID string) (int, error) {
	records, exists := m.records[datasetID]
	if !exists {
		return 0, nil
	}
	return len(records), nil
}

func (m *datasetManagerMockRepo) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

// ========================================
// CreateDataset Tests
// ========================================

func TestDatasetManager_CreateDataset(t *testing.T) {
	repo := newDatasetManagerMockRepo()
	loader := &DatasetLoader{datasetRepo: repo}
	service := NewDatasetManager(repo, loader)

	t.Run("create dataset successfully", func(t *testing.T) {
		req := &CreateDatasetRequest{
			DatasetID:      "test-ds-1",
			Name:           "Test Dataset",
			EvaluationType: EvaluationTypeRAG,
		}

		resp, err := service.CreateDataset(context.Background(), 1, 10, req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "test-ds-1", resp.DatasetID)
		assert.Equal(t, "Test Dataset", resp.Name)
		assert.Equal(t, domeval.DatasetTypeDatabase, resp.Type)
		assert.Equal(t, EvaluationTypeRAG, resp.EvaluationType)
	})

	t.Run("create dataset with description", func(t *testing.T) {
		req := &CreateDatasetRequest{
			DatasetID:      "test-ds-2",
			Name:           "Test Dataset 2",
			Description:    "Test Description",
			EvaluationType: EvaluationTypeAgent,
		}

		resp, err := service.CreateDataset(context.Background(), 1, 10, req)
		assert.NoError(t, err)
		assert.Equal(t, "Test Description", resp.Description)
	})

	t.Run("create dataset with samples", func(t *testing.T) {
		qapairs := []*domeval.QAPair{
			{Question: "Q1", ReferenceAnswer: "A1"},
			{Question: "Q2", ReferenceAnswer: "A2"},
		}
		req := &CreateDatasetRequest{
			DatasetID:      "test-ds-3",
			Name:           "Test Dataset 3",
			EvaluationType: EvaluationTypeRAG,
			QAPairs:        qapairs,
		}

		resp, err := service.CreateDataset(context.Background(), 1, 10, req)
		assert.NoError(t, err)
		assert.Equal(t, 2, resp.QACount)
	})

	t.Run("validate required fields", func(t *testing.T) {
		req := &CreateDatasetRequest{
			// Missing Name
			EvaluationType: EvaluationTypeRAG,
		}

		resp, err := service.CreateDataset(context.Background(), 1, 10, req)
		assert.Error(t, err)
		assert.Nil(t, resp)
	})
}

// ========================================
// GetDataset Tests
// ========================================

func TestDatasetManager_GetDataset(t *testing.T) {
	repo := newDatasetManagerMockRepo()
	loader := &DatasetLoader{datasetRepo: repo}
	service := NewDatasetManager(repo, loader)

	// Setup: Create a dataset
	dataset := domeval.NewDataset("test-ds-1", 1, 10, "Test Dataset", domeval.EvaluationTypeRAG)
	_ = repo.Create(context.Background(), dataset)

	t.Run("get dataset successfully", func(t *testing.T) {
		resp, err := service.GetDataset(context.Background(), "test-ds-1", 1)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "test-ds-1", resp.DatasetID)
		assert.Equal(t, "Test Dataset", resp.Name)
	})

	t.Run("get dataset not found", func(t *testing.T) {
		resp, err := service.GetDataset(context.Background(), "non-existent", 1)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.True(t, errors.Is(err, domeval.ErrDatasetNotFound))
	})

	t.Run("get dataset with wrong tenant", func(t *testing.T) {
		resp, err := service.GetDataset(context.Background(), "test-ds-1", 2)
		assert.Error(t, err)
		assert.Nil(t, resp)
	})
}

// ========================================
// ListDatasets Tests
// ========================================

func TestDatasetManager_ListDatasets(t *testing.T) {
	repo := newDatasetManagerMockRepo()
	loader := &DatasetLoader{datasetRepo: repo}
	service := NewDatasetManager(repo, loader)

	// Setup: Create datasets
	for i := 1; i <= 3; i++ {
		dataset := domeval.NewDataset("ds-list-"+string(rune('a'+i)), 1, 10, "Dataset "+string(rune('a'+i)), domeval.EvaluationTypeRAG)
		_ = repo.Create(context.Background(), dataset)
	}

	t.Run("list all datasets", func(t *testing.T) {
		datasets, err := service.ListDatasets(context.Background(), 1, nil)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(datasets), 3)
	})

	t.Run("list with evaluation type filter", func(t *testing.T) {
		ragType := EvaluationTypeRAG
		datasets, err := service.ListDatasets(context.Background(), 1, &ragType)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(datasets), 3)
	})
}

// ========================================
// UpdateDataset Tests
// ========================================

func TestDatasetManager_UpdateDataset(t *testing.T) {
	repo := newDatasetManagerMockRepo()
	loader := &DatasetLoader{datasetRepo: repo}
	service := NewDatasetManager(repo, loader)

	// Setup: Create a dataset
	dataset := domeval.NewDataset("test-ds-update", 1, 10, "Original Name", domeval.EvaluationTypeRAG)
	_ = repo.Create(context.Background(), dataset)

	t.Run("update dataset successfully", func(t *testing.T) {
		req := &UpdateDatasetRequest{
			Name:        "Updated Name",
			Description: "Updated Description",
		}

		resp, err := service.UpdateDataset(context.Background(), "test-ds-update", 1, req)
		assert.NoError(t, err)
		assert.Equal(t, "Updated Name", resp.Name)
		assert.Equal(t, "Updated Description", resp.Description)
	})

	t.Run("update non-existent dataset", func(t *testing.T) {
		req := &UpdateDatasetRequest{
			Name: "Name",
		}

		resp, err := service.UpdateDataset(context.Background(), "non-existent", 1, req)
		assert.Error(t, err)
		assert.Nil(t, resp)
	})
}

// ========================================
// DeleteDataset Tests
// ========================================

func TestDatasetManager_DeleteDataset(t *testing.T) {
	repo := newDatasetManagerMockRepo()
	loader := &DatasetLoader{datasetRepo: repo}
	service := NewDatasetManager(repo, loader)

	t.Run("delete dataset successfully", func(t *testing.T) {
		// Setup: Create a dataset
		dataset := domeval.NewDataset("test-ds-delete", 1, 10, "Delete Me", domeval.EvaluationTypeRAG)
		_ = repo.Create(context.Background(), dataset)

		err := service.DeleteDataset(context.Background(), "test-ds-delete", 1)
		assert.NoError(t, err)

		// Verify it's deleted
		_, err = repo.FindByIDWithTenant(context.Background(), "test-ds-delete", 1)
		assert.Error(t, err)
	})

	t.Run("delete non-existent dataset", func(t *testing.T) {
		err := service.DeleteDataset(context.Background(), "non-existent", 1)
		assert.Error(t, err)
	})
}

// ========================================
// ListSamples Tests
// ========================================

func TestDatasetManager_ListSamples(t *testing.T) {
	repo := newDatasetManagerMockRepo()
	loader := &DatasetLoader{datasetRepo: repo}
	service := NewDatasetManager(repo, loader)

	// Setup: Create dataset with samples
	dataset := domeval.NewDataset("test-ds-samples", 1, 10, "Dataset", domeval.EvaluationTypeRAG)
	_ = repo.Create(context.Background(), dataset)

	samples := []*domeval.DatasetRecord{
		domeval.NewDatasetRecord("test-ds-samples", 1, "Q1", "A1"),
		domeval.NewDatasetRecord("test-ds-samples", 1, "Q2", "A2"),
		domeval.NewDatasetRecord("test-ds-samples", 1, "Q3", "A3"),
	}
	_ = repo.AddSamples(context.Background(), "test-ds-samples", samples)

	t.Run("list samples successfully", func(t *testing.T) {
		req := &ListSamplesRequest{
			Page:     1,
			PageSize: 10,
		}
		resp, err := service.ListSamples(context.Background(), "test-ds-samples", 1, req)
		assert.NoError(t, err)
		assert.Len(t, resp.Samples, 3)
		assert.Equal(t, int64(3), resp.Total)
	})

	t.Run("list samples with pagination", func(t *testing.T) {
		req := &ListSamplesRequest{
			Page:     1,
			PageSize: 2,
		}
		resp, err := service.ListSamples(context.Background(), "test-ds-samples", 1, req)
		assert.NoError(t, err)
		assert.Len(t, resp.Samples, 2)
		assert.Equal(t, int64(3), resp.Total)
	})

	t.Run("list samples from non-existent dataset", func(t *testing.T) {
		req := &ListSamplesRequest{
			Page:     1,
			PageSize: 10,
		}
		resp, err := service.ListSamples(context.Background(), "non-existent", 1, req)
		// The use case doesn't check if dataset exists before listing samples
		// It returns empty list if dataset doesn't have any samples
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Samples, 0)
	})
}

// ========================================
// AddSamples Tests
// ========================================

func TestDatasetManager_AddSamples(t *testing.T) {
	repo := newDatasetManagerMockRepo()
	loader := &DatasetLoader{datasetRepo: repo}
	service := NewDatasetManager(repo, loader)

	// Setup: Create a dataset
	dataset := domeval.NewDataset("test-ds-add-samples", 1, 10, "Dataset", domeval.EvaluationTypeRAG)
	_ = repo.Create(context.Background(), dataset)

	t.Run("add samples successfully", func(t *testing.T) {
		qapairs := []*domeval.QAPair{
			{Question: "Q1", ReferenceAnswer: "A1"},
			{Question: "Q2", ReferenceAnswer: "A2"},
		}
		req := &AddSamplesRequest{
			QAPairs: qapairs,
		}

		err := service.AddSamples(context.Background(), "test-ds-add-samples", 1, req)
		assert.NoError(t, err)
	})

	t.Run("add samples with relevant PIDs", func(t *testing.T) {
		qapairs := []*domeval.QAPair{
			{
				Question:        "Q1",
				ReferenceAnswer: "A1",
				RelevantPIDs:    []string{"pid1", "pid2"},
			},
		}
		req := &AddSamplesRequest{
			QAPairs: qapairs,
		}

		err := service.AddSamples(context.Background(), "test-ds-add-samples", 1, req)
		assert.NoError(t, err)
	})

	t.Run("add samples to non-existent dataset", func(t *testing.T) {
		qapairs := []*domeval.QAPair{
			{Question: "Q1", ReferenceAnswer: "A1"},
		}
		req := &AddSamplesRequest{
			QAPairs: qapairs,
		}

		err := service.AddSamples(context.Background(), "non-existent", 1, req)
		assert.Error(t, err)
	})

	t.Run("validate empty samples", func(t *testing.T) {
		req := &AddSamplesRequest{
			QAPairs: []*domeval.QAPair{},
		}

		err := service.AddSamples(context.Background(), "test-ds-add-samples", 1, req)
		assert.Error(t, err)
	})
}

// ========================================
// DeleteSample Tests
// ========================================

func TestDatasetManager_DeleteSample(t *testing.T) {
	repo := newDatasetManagerMockRepo()
	loader := &DatasetLoader{datasetRepo: repo}
	service := NewDatasetManager(repo, loader)

	// Setup: Create dataset with samples
	dataset := domeval.NewDataset("test-ds-del-sample", 1, 10, "Dataset", domeval.EvaluationTypeRAG)
	_ = repo.Create(context.Background(), dataset)

	samples := []*domeval.DatasetRecord{
		domeval.NewDatasetRecord("test-ds-del-sample", 1, "Q1", "A1"),
		domeval.NewDatasetRecord("test-ds-del-sample", 1, "Q2", "A2"),
	}
	_ = repo.AddSamples(context.Background(), "test-ds-del-sample", samples)

	t.Run("delete sample successfully", func(t *testing.T) {
		// After creating dataset, nextID is 2. Adding samples gives them IDs 2 and 3.
		err := service.DeleteSample(context.Background(), "test-ds-del-sample", 1, 2)
		assert.NoError(t, err)
	})

	t.Run("delete sample from non-existent dataset", func(t *testing.T) {
		err := service.DeleteSample(context.Background(), "non-existent", 1, 1)
		assert.Error(t, err)
	})

	t.Run("delete non-existent sample", func(t *testing.T) {
		err := service.DeleteSample(context.Background(), "test-ds-del-sample", 1, 999)
		assert.Error(t, err)
	})
}
