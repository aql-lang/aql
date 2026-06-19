//go:build !js

package native

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoKVStore is a KeyValBackend backed by a MongoDB collection: each
// key is a document _id with the value stored as a JSON string in a
// `data` field (shared codec). Opened with a mongodb:// URL whose path
// names the database; an optional ?collection= query selects the
// collection (default "kv").
type MongoKVStore struct {
	client *mongo.Client
	coll   *mongo.Collection
}

var _ KeyValBackend = (*MongoKVStore)(nil)

const kvMongoTimeout = 10 * time.Second

// mongoKVDoc is the stored document shape.
type mongoKVDoc struct {
	ID   string `bson:"_id"`
	Data string `bson:"data"`
}

// NewMongoKVStore connects to MongoDB and selects the database (from the
// URL path) and collection (from ?collection=, default "kv").
func NewMongoKVStore(dsn string) (*MongoKVStore, error) {
	dbName, collName, err := parseMongoDSN(dsn)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), kvMongoTimeout)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(dsn))
	if err != nil {
		return nil, fmt.Errorf("mongo connect: %w", redactError(err, dsn))
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("mongo connect: %w", redactError(err, dsn))
	}
	return &MongoKVStore{client: client, coll: client.Database(dbName).Collection(collName)}, nil
}

// parseMongoDSN extracts the database and collection from a mongodb URL.
func parseMongoDSN(dsn string) (db, coll string, err error) {
	u, perr := url.Parse(dsn)
	if perr != nil {
		return "", "", fmt.Errorf("mongo url: %w", redactError(perr, dsn))
	}
	db = strings.TrimPrefix(u.Path, "/")
	if db == "" {
		return "", "", errors.New("mongo url must include a database path, e.g. mongodb://host/mydb")
	}
	coll = u.Query().Get("collection")
	if coll == "" {
		coll = "kv"
	}
	return db, coll, nil
}

func (s *MongoKVStore) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), kvMongoTimeout)
}

func (s *MongoKVStore) Get(key string) (Value, bool, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	var doc mongoKVDoc
	err := s.coll.FindOne(ctx, bson.M{"_id": key}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Value{}, false, nil
	}
	if err != nil {
		return Value{}, false, err
	}
	v, err := kvBytesToValue([]byte(doc.Data))
	if err != nil {
		return Value{}, false, err
	}
	return v, true, nil
}

func (s *MongoKVStore) Set(key string, val Value) error {
	b, err := valueToKVBytes(val)
	if err != nil {
		return err
	}
	ctx, cancel := s.ctx()
	defer cancel()
	_, err = s.coll.ReplaceOne(ctx,
		bson.M{"_id": key},
		mongoKVDoc{ID: key, Data: string(b)},
		options.Replace().SetUpsert(true))
	return err
}

func (s *MongoKVStore) Delete(key string) (bool, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	res, err := s.coll.DeleteOne(ctx, bson.M{"_id": key})
	if err != nil {
		return false, err
	}
	return res.DeletedCount > 0, nil
}

func (s *MongoKVStore) Has(key string) (bool, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	n, err := s.coll.CountDocuments(ctx, bson.M{"_id": key})
	return n > 0, err
}

func (s *MongoKVStore) Keys(prefix string) ([]string, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	filter := bson.M{}
	if prefix != "" {
		filter = bson.M{"_id": bson.M{"$regex": "^" + regexp.QuoteMeta(prefix)}}
	}
	cur, err := s.coll.Find(ctx, filter, options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []string
	for cur.Next(ctx) {
		var doc struct {
			ID string `bson:"_id"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, doc.ID)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

func (s *MongoKVStore) Kind() string { return "mongo" }

func (s *MongoKVStore) Close() error {
	ctx, cancel := s.ctx()
	defer cancel()
	return s.client.Disconnect(ctx)
}
