package cluster

import (
	"bytes"
	"encoding/json"
	"strings"

	"go.pedge.io/dlog"

	"github.com/libopenstorage/openstorage/api"
	"github.com/portworx/kvdb"
)

const (
	ClusterDBKey = "cluster/database"
)

func readDatabase() (Database, error) {
	kvdb := kvdb.Instance()

	db := Database{
		Status:      api.Status_STATUS_INIT,
		NodeEntries: make(map[string]NodeEntry),
	}

	kv, err := kvdb.Get(ClusterDBKey)
	if err != nil && !strings.Contains(err.Error(), "Key not found") {
		dlog.Warnln("Warning, could not read cluster database")
		return db, err
	}

	if kv == nil || bytes.Compare(kv.Value, []byte("{}")) == 0 {
		dlog.Infoln("Cluster is uninitialized...")
		return db, nil
	}
	if err := json.Unmarshal(kv.Value, &db); err != nil {
		dlog.Warnln("Fatal, Could not parse cluster database ", kv)
		return db, err
	}

	return db, nil
}

func writeDatabase(db *Database) error {
	kvdb := kvdb.Instance()
	b, err := json.Marshal(db)
	if err != nil {
		dlog.Warnf("Fatal, Could not marshal cluster database to JSON: %v", err)
		return err
	}

	if _, err := kvdb.Put(ClusterDBKey, b, 0); err != nil {
		dlog.Warnf("Fatal, Could not marshal cluster database to JSON: %v", err)
		return err
	}

	dlog.Infoln("Cluster database updated.")
	return nil
}
