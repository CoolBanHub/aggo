package database

import (
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/components/retriever"
)

type Database interface {
	indexer.Indexer

	retriever.Retriever
}
