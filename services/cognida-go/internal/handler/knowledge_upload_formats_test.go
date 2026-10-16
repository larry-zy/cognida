package handler

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestKnowledgeUploadSupportedFormatContract(t *testing.T) {
	for _, name := range []string{"a.txt", "a.MD", "a.markdown", "a.pdf", "a.docx", "a.xlsx", "a.csv", "a.html", "a.htm", "a.pptx"} {
		require.NoError(t, validateKnowledgeFileFormat(name), name)
	}
	for _, name := range []string{"old.doc", "old.xls", "old.ppt", "unknown.exe", "no-extension", "file.docx.exe"} {
		require.Error(t, validateKnowledgeFileFormat(name), name)
	}
}

func TestKnowledgeUploadRejectsUnsupportedFormatBeforeStorageHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, name := range []string{"legacy.doc", "legacy.xls", "legacy.ppt", "unknown.exe"} {
		t.Run(name, func(t *testing.T) {
			var body bytes.Buffer
			form := multipart.NewWriter(&body)
			part, err := form.CreateFormFile("file", name)
			require.NoError(t, err)
			_, err = part.Write([]byte("not parsed or persisted"))
			require.NoError(t, err)
			require.NoError(t, form.Close())
			router := gin.New()
			// Nil downstream ports prove invalid formats never reach persistence/parser.
			h := NewKnowledgeBaseHandler(nil, nil, nil, nil)
			router.POST("/knowledge-bases/:id/knowledge/file", h.UploadKnowledgeFile)
			req := httptest.NewRequest("POST", "/knowledge-bases/short/knowledge/file", &body)
			req.Header.Set("Content-Type", form.FormDataContentType())
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, 400, w.Code)
		})
	}
}
