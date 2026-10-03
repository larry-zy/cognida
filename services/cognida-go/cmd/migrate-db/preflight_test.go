package main

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestPreflightCollectionPreservesUnversionedTables(t *testing.T) {
	for _, version := range []uint{0, 3, 4} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		mock.ExpectQuery("SELECT table_name FROM information_schema.tables").WillReturnRows(sqlmock.NewRows([]string{"table_name"}).AddRow("collection_tasks"))
		err = preflightCollection(context.Background(), db, version)
		require.ErrorContains(t, err, "no migration has been applied")
		require.ErrorContains(t, err, "collection_tasks")
		mock.ExpectClose()
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	}
}

func TestPreflightCollectionAllowsFreshAndVersionedSchemas(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	for _, version := range []uint{0, 3, 4} {
		mock.ExpectQuery("SELECT table_name FROM information_schema.tables").WillReturnRows(sqlmock.NewRows([]string{"table_name"}))
		require.NoError(t, preflightCollection(context.Background(), db, version))
	}
	// A migrated schema never gets mistaken for a legacy schema.
	require.NoError(t, preflightCollection(context.Background(), db, 5))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPreflightCollectionFailsClosedOnIntrospectionError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery("SELECT table_name FROM information_schema.tables").WillReturnError(errors.New("unavailable"))
	require.ErrorContains(t, preflightCollection(context.Background(), db, 3), "unavailable")
	require.NoError(t, mock.ExpectationsWereMet())
}
