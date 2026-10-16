package wire

import (
	"cognida/internal/handler"
	k "cognida/internal/model/knowledge"
	"cognida/internal/model/rag"
	"cognida/internal/repository/mysql"
	"cognida/internal/service/evaluation"
	"cognida/internal/service/feedback"
	"cognida/internal/service/knowledge"
	"fmt"
	"gorm.io/gorm"
)

func (f *FeatureServices) configureFeedback(db *gorm.DB, kbs k.KnowledgeBaseRepository, docs k.KnowledgeRepository, chunks k.ChunkRepository, processor knowledge.DocumentProcessorService, vectors k.VectorRepository, kbService knowledge.KnowledgeBaseService, eval *evaluation.Service, retriever rag.Retriever, capability *knowledge.RetrievalCapability, kbHandler *handler.KnowledgeBaseHandler) error {
	indices, ok := vectors.(knowledge.FeedbackProjectionReader)
	if !ok {
		return fmt.Errorf("feedback requires a queryable publication index")
	}
	projection := knowledge.NewFeedbackProjection(kbs, docs, chunks, processor, indices, kbService, f.Governance)
	service, err := feedback.NewService(mysql.NewFeedbackRepository(db), f.Lifecycle, projection, eval, f.TaskControl, f.TaskRuntime, featureAuthorizer(db))
	if err != nil {
		return err
	}
	guarded, ok := retriever.(interface{ SetConsumptionGuard(rag.ConsumptionGuard) })
	if !ok {
		return fmt.Errorf("feedback requires a guarded knowledge retriever")
	}
	f.Feedback = service
	f.FeedbackWorker = feedback.NewWorker(service)
	f.FeedbackHandler = handler.NewFeedbackHandler(service)
	f.EvaluationSnapshots.FeedbackView = service.CheckView
	guarded.SetConsumptionGuard(service)
	capability.SetConsumptionGuard(service)
	kbHandler.SetKnowledgeConsumptionGuard(service.KnowledgeAccess)
	return nil
}
