// Package mysql provides DatasetRepository unit tests
package mysql

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	domeval "link/internal/model/evaluation"
)

// ========================================
// Test Helpers
// ========================================

func createTestDataset(datasetID, name string, tenantID int64) *domeval.Dataset {
	return domeval.NewDataset(datasetID, tenantID, 1, name, domeval.EvaluationTypeRAG)
}

// ========================================
// Mock DatasetRepository for Testing
// ========================================

// mockDatasetRepository 内存实现的 DatasetRepository，用于测试
type mockDatasetRepository struct {
	datasets map[string]*domeval.Dataset // datasetID -> dataset
	records  map[string][]*domeval.DatasetRecord // datasetID -> records
	nextID   int64
}

func newMockDatasetRepository() *mockDatasetRepository {
	return &mockDatasetRepository{
		datasets: make(map[string]*domeval.Dataset),
		records:  make(map[string][]*domeval.DatasetRecord),
		nextID:   1,
	}
}

func (m *mockDatasetRepository) Create(ctx context.Context, dataset *domeval.Dataset) error {
	m.datasets[dataset.DatasetID] = dataset
	return nil
}

func (m *mockDatasetRepository) FindByID(ctx context.Context, id string) (*domeval.Dataset, error) {
	ds, ok := m.datasets[id]
	if !ok || ds.DeletedAt != nil {
		return nil, domeval.ErrDatasetNotFound
	}
	return ds, nil
}

func (m *mockDatasetRepository) FindByIDWithTenant(ctx context.Context, id string, tenantID int64) (*domeval.Dataset, error) {
	ds, ok := m.datasets[id]
	if !ok || ds.DeletedAt != nil {
		return nil, domeval.ErrDatasetNotFound
	}
	if ds.TenantID != tenantID {
		return nil, domeval.ErrDatasetNotFound
	}
	return ds, nil
}

func (m *mockDatasetRepository) List(ctx context.Context, filter *domeval.DatasetFilter) ([]*domeval.Dataset, int64, error) {
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
		if filter.Search != "" && !contains(ds.Name, filter.Search) {
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

func (m *mockDatasetRepository) Update(ctx context.Context, dataset *domeval.Dataset) error {
	ds, ok := m.datasets[dataset.DatasetID]
	if !ok || ds.DeletedAt != nil {
		return domeval.ErrDatasetNotFound
	}
	ds.Name = dataset.Name
	ds.Description = dataset.Description
	ds.Type = dataset.Type
	ds.EvaluationType = dataset.EvaluationType
	ds.UpdatedAt = time.Now()
	return nil
}

func (m *mockDatasetRepository) SoftDelete(ctx context.Context, id string, tenantID int64) error {
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

func (m *mockDatasetRepository) AddSamples(ctx context.Context, datasetID string, samples []*domeval.DatasetRecord) error {
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

	// Update QA count
	if ds, ok := m.datasets[datasetID]; ok {
		ds.QACount += len(samples)
	}
	return nil
}

func (m *mockDatasetRepository) ListSamples(ctx context.Context, datasetID string, tenantID int64, page, pageSize int) ([]*domeval.DatasetRecord, int64, error) {
	records, exists := m.records[datasetID]
	if !exists {
		return []*domeval.DatasetRecord{}, 0, nil
	}

	// Filter by tenantID
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

func (m *mockDatasetRepository) DeleteSample(ctx context.Context, datasetID string, tenantID int64, sampleID int64) error {
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

			// Update QA count
			if ds, ok := m.datasets[datasetID]; ok && ds.QACount > 0 {
				ds.QACount--
			}
			return nil
		}
	}
	return domeval.ErrDatasetSampleNotFound
}

func (m *mockDatasetRepository) CountSamples(ctx context.Context, datasetID string) (int, error) {
	records, exists := m.records[datasetID]
	if !exists {
		return 0, nil
	}
	return len(records), nil
}

// Helper function for string contains
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ========================================
// Create Tests
// ========================================

func TestMockDatasetRepository_Create(t *testing.T) {
	repo := newMockDatasetRepository()

	t.Run("create dataset successfully", func(t *testing.T) {
		dataset := createTestDataset("test-ds-1", "Test Dataset", 1)

		err := repo.Create(context.Background(), dataset)
		assert.NoError(t, err)
		assert.Len(t, repo.datasets, 1)
	})
}

// ========================================
// FindByID Tests
// ========================================

func TestMockDatasetRepository_FindByID(t *testing.T) {
	repo := newMockDatasetRepository()

	t.Run("find by id successfully", func(t *testing.T) {
		dataset := createTestDataset("test-ds-1", "Test Dataset", 1)
		_ = repo.Create(context.Background(), dataset)

		found, err := repo.FindByID(context.Background(), "test-ds-1")
		assert.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, "test-ds-1", found.DatasetID)
		assert.Equal(t, "Test Dataset", found.Name)
	})

	t.Run("find by id not found", func(t *testing.T) {
		found, err := repo.FindByID(context.Background(), "non-existent")
		assert.Error(t, err)
		assert.Nil(t, found)
		assert.Equal(t, domeval.ErrDatasetNotFound, err)
	})

	t.Run("find by id with soft deleted", func(t *testing.T) {
		dataset := createTestDataset("test-ds-2", "Test Dataset 2", 1)
		_ = repo.Create(context.Background(), dataset)
		_ = repo.SoftDelete(context.Background(), "test-ds-2", 1)

		found, err := repo.FindByID(context.Background(), "test-ds-2")
		assert.Error(t, err)
		assert.Nil(t, found)
		assert.Equal(t, domeval.ErrDatasetNotFound, err)
	})
}

// ========================================
// FindByIDWithTenant Tests
// ========================================

func TestMockDatasetRepository_FindByIDWithTenant(t *testing.T) {
	repo := newMockDatasetRepository()

	t.Run("find by id and tenant successfully", func(t *testing.T) {
		dataset := createTestDataset("test-ds-tenant-1", "Test Dataset", 1)
		_ = repo.Create(context.Background(), dataset)

		found, err := repo.FindByIDWithTenant(context.Background(), "test-ds-tenant-1", 1)
		assert.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, "test-ds-tenant-1", found.DatasetID)
	})

	t.Run("find by id with wrong tenant", func(t *testing.T) {
		dataset := createTestDataset("test-ds-tenant-2", "Test Dataset", 1)
		_ = repo.Create(context.Background(), dataset)

		found, err := repo.FindByIDWithTenant(context.Background(), "test-ds-tenant-2", 2)
		assert.Error(t, err)
		assert.Nil(t, found)
		assert.Equal(t, domeval.ErrDatasetNotFound, err)
	})
}

// ========================================
// List Tests
// ========================================

func TestMockDatasetRepository_List(t *testing.T) {
	t.Run("list all datasets", func(t *testing.T) {
		repo := newMockDatasetRepository()
		for i := 0; i < 5; i++ {
			dataset := createTestDataset("ds-list-all-"+string(rune('a'+i)), "Dataset "+string(rune('a'+i)), 1)
			_ = repo.Create(context.Background(), dataset)
		}

		filter := &domeval.DatasetFilter{
			Page:     1,
			PageSize: 10,
		}
		datasets, total, err := repo.List(context.Background(), filter)
		assert.NoError(t, err)
		assert.Len(t, datasets, 5)
		assert.Equal(t, int64(5), total)
	})

	t.Run("list with tenant filter", func(t *testing.T) {
		repo := newMockDatasetRepository()
		dataset1 := createTestDataset("ds-tenant-1", "Tenant 1 Dataset", 1)
		dataset2 := createTestDataset("ds-tenant-2", "Tenant 2 Dataset", 2)
		_ = repo.Create(context.Background(), dataset1)
		_ = repo.Create(context.Background(), dataset2)

		tenantID := int64(1)
		filter := &domeval.DatasetFilter{
			TenantID: &tenantID,
			Page:     1,
			PageSize: 10,
		}
		datasets, total, err := repo.List(context.Background(), filter)
		assert.NoError(t, err)
		assert.Len(t, datasets, 1)
		assert.Equal(t, int64(1), total)
		assert.Equal(t, "ds-tenant-1", datasets[0].DatasetID)
	})

	t.Run("list with type filter", func(t *testing.T) {
		repo := newMockDatasetRepository()
		dataset1 := createTestDataset("ds-type-1", "Type 1 Dataset", 1)
		dataset1.Type = domeval.DatasetTypeDatabase
		dataset2 := createTestDataset("ds-type-2", "Type 2 Dataset", 1)
		dataset2.Type = domeval.DatasetTypeFile
		_ = repo.Create(context.Background(), dataset1)
		_ = repo.Create(context.Background(), dataset2)

		dbType := domeval.DatasetTypeDatabase
		filter := &domeval.DatasetFilter{
			Type:     &dbType,
			Page:     1,
			PageSize: 10,
		}
		datasets, _, err := repo.List(context.Background(), filter)
		assert.NoError(t, err)
		assert.Len(t, datasets, 1)
		assert.Equal(t, domeval.DatasetTypeDatabase, datasets[0].Type)
	})

	t.Run("list with search filter", func(t *testing.T) {
		repo := newMockDatasetRepository()
		dataset1 := createTestDataset("ds-search-1", "Test Dataset Alpha", 1)
		dataset2 := createTestDataset("ds-search-2", "Production Dataset", 1)
		_ = repo.Create(context.Background(), dataset1)
		_ = repo.Create(context.Background(), dataset2)

		filter := &domeval.DatasetFilter{
			Search:   "Test",
			Page:     1,
			PageSize: 10,
		}
		datasets, _, err := repo.List(context.Background(), filter)
		assert.NoError(t, err)
		assert.Len(t, datasets, 1)
		assert.Equal(t, "Test Dataset Alpha", datasets[0].Name)
	})

	t.Run("list with pagination", func(t *testing.T) {
		repo := newMockDatasetRepository()
		for i := 0; i < 15; i++ {
			dataset := createTestDataset("ds-page-"+string(rune('a'+i%26)), "Dataset", 1)
			_ = repo.Create(context.Background(), dataset)
		}

		filter := &domeval.DatasetFilter{
			Page:     1,
			PageSize: 10,
		}
		datasets, total, err := repo.List(context.Background(), filter)
		assert.NoError(t, err)
		assert.Len(t, datasets, 10)
		assert.Equal(t, int64(15), total)
	})
}

// ========================================
// Update Tests
// ========================================

func TestMockDatasetRepository_Update(t *testing.T) {
	repo := newMockDatasetRepository()

	t.Run("update dataset successfully", func(t *testing.T) {
		dataset := createTestDataset("ds-update-1", "Original Name", 1)
		dataset.Description = "Original Description"
		_ = repo.Create(context.Background(), dataset)

		dataset.Name = "Updated Name"
		dataset.Description = "Updated Description"

		err := repo.Update(context.Background(), dataset)
		assert.NoError(t, err)

		found, _ := repo.FindByID(context.Background(), "ds-update-1")
		assert.Equal(t, "Updated Name", found.Name)
		assert.Equal(t, "Updated Description", found.Description)
	})

	t.Run("update non-existent dataset", func(t *testing.T) {
		dataset := createTestDataset("non-existent", "Name", 1)

		err := repo.Update(context.Background(), dataset)
		assert.Error(t, err)
		assert.Equal(t, domeval.ErrDatasetNotFound, err)
	})
}

// ========================================
// SoftDelete Tests
// ========================================

func TestMockDatasetRepository_SoftDelete(t *testing.T) {
	repo := newMockDatasetRepository()

	t.Run("soft delete successfully", func(t *testing.T) {
		dataset := createTestDataset("ds-delete-1", "Delete Me", 1)
		_ = repo.Create(context.Background(), dataset)

		err := repo.SoftDelete(context.Background(), "ds-delete-1", 1)
		assert.NoError(t, err)

		found, err := repo.FindByID(context.Background(), "ds-delete-1")
		assert.Error(t, err)
		assert.Nil(t, found)
	})

	t.Run("soft delete non-existent dataset", func(t *testing.T) {
		err := repo.SoftDelete(context.Background(), "non-existent", 1)
		assert.Error(t, err)
		assert.Equal(t, domeval.ErrDatasetNotFound, err)
	})

	t.Run("soft delete with wrong tenant", func(t *testing.T) {
		dataset := createTestDataset("ds-delete-2", "Delete Me", 1)
		_ = repo.Create(context.Background(), dataset)

		err := repo.SoftDelete(context.Background(), "ds-delete-2", 2)
		assert.Error(t, err)
		assert.Equal(t, domeval.ErrDatasetNotFound, err)
	})
}

// ========================================
// AddSamples Tests
// ========================================

func TestMockDatasetRepository_AddSamples(t *testing.T) {
	repo := newMockDatasetRepository()

	t.Run("add samples successfully", func(t *testing.T) {
		dataset := createTestDataset("ds-samples-1", "Dataset with Samples", 1)
		_ = repo.Create(context.Background(), dataset)

		records := []*domeval.DatasetRecord{
			domeval.NewDatasetRecord("ds-samples-1", 1, "Question 1", "Answer 1"),
			domeval.NewDatasetRecord("ds-samples-1", 1, "Question 2", "Answer 2"),
		}

		err := repo.AddSamples(context.Background(), "ds-samples-1", records)
		assert.NoError(t, err)
		assert.Len(t, repo.records["ds-samples-1"], 2)

		// Check QA count updated
		found, _ := repo.FindByID(context.Background(), "ds-samples-1")
		assert.Equal(t, 2, found.QACount)
	})

	t.Run("add samples to non-existent dataset", func(t *testing.T) {
		records := []*domeval.DatasetRecord{
			domeval.NewDatasetRecord("non-existent", 1, "Q", "A"),
		}

		err := repo.AddSamples(context.Background(), "non-existent", records)
		assert.Error(t, err)
		assert.Equal(t, domeval.ErrDatasetNotFound, err)
	})
}

// ========================================
// ListSamples Tests
// ========================================

func TestMockDatasetRepository_ListSamples(t *testing.T) {
	repo := newMockDatasetRepository()

	t.Run("list samples successfully", func(t *testing.T) {
		dataset := createTestDataset("ds-list-1", "Dataset", 1)
		_ = repo.Create(context.Background(), dataset)

		records := []*domeval.DatasetRecord{
			domeval.NewDatasetRecord("ds-list-1", 1, "Q1", "A1"),
			domeval.NewDatasetRecord("ds-list-1", 1, "Q2", "A2"),
			domeval.NewDatasetRecord("ds-list-1", 1, "Q3", "A3"),
		}
		_ = repo.AddSamples(context.Background(), "ds-list-1", records)

		samples, total, err := repo.ListSamples(context.Background(), "ds-list-1", 1, 1, 10)
		assert.NoError(t, err)
		assert.Len(t, samples, 3)
		assert.Equal(t, int64(3), total)
	})

	t.Run("list samples with pagination", func(t *testing.T) {
		dataset := createTestDataset("ds-list-2", "Dataset", 1)
		_ = repo.Create(context.Background(), dataset)

		for i := 0; i < 15; i++ {
			records := []*domeval.DatasetRecord{
				domeval.NewDatasetRecord("ds-list-2", 1, "Q", "A"),
			}
			_ = repo.AddSamples(context.Background(), "ds-list-2", records)
		}

		samples, total, err := repo.ListSamples(context.Background(), "ds-list-2", 1, 1, 10)
		assert.NoError(t, err)
		assert.Len(t, samples, 10)
		assert.Equal(t, int64(15), total)
	})

	t.Run("list samples filters by tenant", func(t *testing.T) {
		dataset1 := createTestDataset("ds-tenant-samples-1", "Dataset 1", 1)
		dataset2 := createTestDataset("ds-tenant-samples-2", "Dataset 2", 2)
		_ = repo.Create(context.Background(), dataset1)
		_ = repo.Create(context.Background(), dataset2)

		records1 := []*domeval.DatasetRecord{
			domeval.NewDatasetRecord("ds-tenant-samples-1", 1, "Q1", "A1"),
		}
		records2 := []*domeval.DatasetRecord{
			domeval.NewDatasetRecord("ds-tenant-samples-2", 2, "Q2", "A2"),
		}
		_ = repo.AddSamples(context.Background(), "ds-tenant-samples-1", records1)
		_ = repo.AddSamples(context.Background(), "ds-tenant-samples-2", records2)

		samples, total, err := repo.ListSamples(context.Background(), "ds-tenant-samples-1", 1, 1, 10)
		assert.NoError(t, err)
		assert.Len(t, samples, 1)
		assert.Equal(t, int64(1), total)
		assert.Equal(t, int64(1), samples[0].TenantID)
	})

	t.Run("list samples from non-existent dataset", func(t *testing.T) {
		samples, total, err := repo.ListSamples(context.Background(), "non-existent", 1, 1, 10)
		assert.NoError(t, err)
		assert.Len(t, samples, 0)
		assert.Equal(t, int64(0), total)
	})
}

// ========================================
// DeleteSample Tests
// ========================================

func TestMockDatasetRepository_DeleteSample(t *testing.T) {
	repo := newMockDatasetRepository()

	t.Run("delete sample successfully", func(t *testing.T) {
		dataset := createTestDataset("ds-del-sample-1", "Dataset", 1)
		_ = repo.Create(context.Background(), dataset)

		records := []*domeval.DatasetRecord{
			domeval.NewDatasetRecord("ds-del-sample-1", 1, "Q1", "A1"),
			domeval.NewDatasetRecord("ds-del-sample-1", 1, "Q2", "A2"),
		}
		_ = repo.AddSamples(context.Background(), "ds-del-sample-1", records)

		// Delete first sample (ID will be 1 after AddSamples)
		err := repo.DeleteSample(context.Background(), "ds-del-sample-1", 1, 1)
		assert.NoError(t, err)

		samples, _, _ := repo.ListSamples(context.Background(), "ds-del-sample-1", 1, 1, 10)
		assert.Len(t, samples, 1)
	})

	t.Run("delete sample from non-existent dataset", func(t *testing.T) {
		err := repo.DeleteSample(context.Background(), "non-existent", 1, 1)
		assert.Error(t, err)
		assert.Equal(t, domeval.ErrDatasetSampleNotFound, err)
	})

	t.Run("delete sample with wrong tenant", func(t *testing.T) {
		dataset := createTestDataset("ds-del-sample-2", "Dataset", 1)
		_ = repo.Create(context.Background(), dataset)

		records := []*domeval.DatasetRecord{
			domeval.NewDatasetRecord("ds-del-sample-2", 1, "Q1", "A1"),
		}
		_ = repo.AddSamples(context.Background(), "ds-del-sample-2", records)

		err := repo.DeleteSample(context.Background(), "ds-del-sample-2", 2, 1)
		assert.Error(t, err)
		assert.Equal(t, domeval.ErrDatasetSampleNotFound, err)
	})
}

// ========================================
// CountSamples Tests
// ========================================

func TestMockDatasetRepository_CountSamples(t *testing.T) {
	repo := newMockDatasetRepository()

	t.Run("count samples successfully", func(t *testing.T) {
		dataset := createTestDataset("ds-count-1", "Dataset", 1)
		_ = repo.Create(context.Background(), dataset)

		for i := 0; i < 5; i++ {
			records := []*domeval.DatasetRecord{
				domeval.NewDatasetRecord("ds-count-1", 1, "Q", "A"),
			}
			_ = repo.AddSamples(context.Background(), "ds-count-1", records)
		}

		count, err := repo.CountSamples(context.Background(), "ds-count-1")
		assert.NoError(t, err)
		assert.Equal(t, 5, count)
	})

	t.Run("count samples for non-existent dataset", func(t *testing.T) {
		count, err := repo.CountSamples(context.Background(), "non-existent")
		assert.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}
