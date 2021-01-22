package daisymanager

import (
	"fmt"
	"github.com/MakeNowJust/heredoc"
	"github.com/go-logr/logr"
	errors2 "k8s.io/apimachinery/pkg/util/errors"
	"strings"

	"github.com/daisy/daisy-operator/api/v1"
	"github.com/daisy/daisy-operator/pkg/daisy"
)

const (
	// Comma-separated ''-enclosed list of database names to be ignored
	// ignoredDBs = "'system'"

	// Max number of tries for SQL queries
	defaultMaxTries = 10

	defaultChUsername = "default"
	defaultChPassword = ""
	defaultChPort     = 8123
)

// Schemer
type Schemer struct {
	Username string
	Password string
	Port     int
	Pool     daisy.ConnectionPool
	Log      logr.Logger
}

// NewSchemer
func NewSchemer(username, password string, port int, logger logr.Logger) *Schemer {
	if len(username) == 0 && len(password) == 0 {
		username = defaultChUsername
		password = defaultChPassword
	}

	if port == 0 {
		port = defaultChPort
	}

	return &Schemer{
		Username: username,
		Password: password,
		Port:     port,
		Log:      logger.WithName("schemer"),
		Pool:     daisy.RealConnectionPool,
	}
}

// getCHConnection
func (s *Schemer) getCHConnection(hostname string) daisy.Connection {
	return s.Pool.GetPooledDBConnection(daisy.NewCHConnectionParams(hostname, s.Username, s.Password, s.Port))
}

// getObjectListFromClickHouse
func (s *Schemer) getObjectListFromClickHouse(endpoints []string, sql string) ([]string, []string, error) {
	log := s.Log

	if len(endpoints) == 0 {
		// Nowhere to fetch data from
		return nil, nil, nil
	}

	// Results
	var names []string
	var statements []string
	var err error

	// Fetch data from any of specified services
	var query *daisy.Query = nil
	for _, endpoint := range endpoints {
		log.V(1).Info("Run query", "Replica", endpoint, "Replicas", endpoints, "SQL", sql)

		query, err = s.getCHConnection(endpoint).Query(sql)
		if err == nil {
			// One of specified services returned result, no need to iterate more
			break
		} else {
			log.V(1).Error(err, "Run query failed", "Replica", endpoint, "Replicas", endpoints)
		}
	}
	if err != nil {
		log.V(1).Info("Run query FAILED on all replicas", "Replicas", endpoints)
		return nil, nil, err
	}

	// Some data available, let's fetch it
	defer query.Close()

	if query.Rows != nil {
		for query.Rows.Next() {
			var name, statement string
			if err := query.Rows.Scan(&name, &statement); err == nil {
				names = append(names, name)
				statements = append(statements, statement)
			} else {
				log.V(1).Error(err, "UNABLE to scan row")
			}
		}
	}

	return names, statements, nil
}

// getCreateDistributedObjects returns a list of objects that needs to be created on a shard in a cluster
// That includes all Distributed tables, corresponding local tables, and databases, if necessary
func (s *Schemer) getCreateDistributedObjects(ctx *memberContext, di *v1.DaisyInstallation, r *v1.Replica) ([]string, []string, error) {
	log := s.Log
	hosts := CreatePodFQDNsOfCluster(ctx.CurCluster, di)
	nHosts := len(hosts)
	if nHosts <= 1 {
		log.V(1).Info("Single host in a cluster. Nothing to create a schema from.")
		return nil, nil, nil
	}

	var hostIndex int
	for i, h := range hosts {
		if h == CreatePodHostname(r) {
			hostIndex = i
			break
		}
	}

	// remove new host from the list. See https://stackoverflow.com/questions/37334119/how-to-delete-an-element-from-a-slice-in-golang
	hosts[hostIndex] = hosts[nHosts-1]
	hosts = hosts[:nHosts-1]
	log.V(1).Info("Extracting distributed table definitions from replicas", "Replicas", hosts)

	clusterTables := fmt.Sprintf("remote('%s', system, tables)", strings.Join(hosts, ","))

	sqlDBs := heredoc.Doc(strings.ReplaceAll(`
		SELECT DISTINCT 
			database AS name, 
			concat('CREATE DATABASE IF NOT EXISTS "', name, '"') AS create_query
		FROM 
		(
			SELECT DISTINCT arrayJoin([database, extract(engine_full, 'Distributed\\([^,]+, *\'?([^,\']+)\'?, *[^,]+')]) database
			FROM cluster('all-sharded', system.tables) tables
			WHERE engine = 'Distributed'
			SETTINGS skip_unavailable_shards = 1
		)`,
		"cluster('all-sharded', system.tables)",
		clusterTables,
	))
	sqlTables := heredoc.Doc(strings.ReplaceAll(`
		SELECT DISTINCT 
			concat(database,'.', name) as name, 
			replaceRegexpOne(create_table_query, 'CREATE (TABLE|VIEW|MATERIALIZED VIEW)', 'CREATE \\1 IF NOT EXISTS')
		FROM 
		(
			SELECT 
			    database, name,
				create_table_query,
				2 AS order
			FROM cluster('all-sharded', system.tables) tables
			WHERE engine = 'Distributed'
			SETTINGS skip_unavailable_shards = 1
			UNION ALL
			SELECT 
				extract(engine_full, 'Distributed\\([^,]+, *\'?([^,\']+)\'?, *[^,]+') AS database, 
				extract(engine_full, 'Distributed\\([^,]+, [^,]+, *\'?([^,\\\')]+)') AS name,
				t.create_table_query,
				1 AS order
			FROM cluster('all-sharded', system.tables) tables
			LEFT JOIN (SELECT distinct database, name, create_table_query 
			             FROM cluster('all-sharded', system.tables) SETTINGS skip_unavailable_shards = 1)  t USING (database, name)
			WHERE engine = 'Distributed' AND t.create_table_query != ''
			SETTINGS skip_unavailable_shards = 1
		) tables
		ORDER BY order
		`,
		"cluster('all-sharded', system.tables)",
		clusterTables,
	))

	log.V(1).Info("fetch dbs list")
	log.V(1).Info("dbs sql", "SQL", sqlDBs)
	names1, sqlStatements1, _ := s.getObjectListFromClickHouse(CreatePodFQDNsOfInstallation(di), sqlDBs)
	log.V(1).Info("DBNames:")
	for _, v := range names1 {
		log.V(1).Info("query result: ", "DBName", v)
	}
	log.V(1).Info("DB Objects:")
	for _, v := range sqlStatements1 {
		log.V(1).Info("query result: ", "DB", v)
	}

	log.V(1).Info("fetch table list")
	log.V(1).Info("tbl sql", "SQL", sqlTables)
	names2, sqlStatements2, _ := s.getObjectListFromClickHouse(CreatePodFQDNsOfInstallation(di), sqlTables)
	log.V(1).Info("Table Names:")
	for _, v := range names2 {
		log.V(1).Info("query result: ", "TableName", v)
	}
	log.V(1).Info("Table Objects:")
	for _, v := range sqlStatements2 {
		log.V(1).Info("query result: ", "Table", v)
	}

	return append(names1, names2...), append(sqlStatements1, sqlStatements2...), nil
}

// getCreateReplicaObjects returns a list of objects that needs to be created on a host in a cluster
func (s *Schemer) getCreateReplicaObjects(ctx *memberContext, di *v1.DaisyInstallation) ([]string, []string, error) {
	log := s.Log

	replicas := GetNormalPodFQDNsOfShard(ctx.CurCluster, ctx.CurShard, di)
	nReplicas := len(replicas)
	replicaCount := di.Spec.Configuration.Clusters[ctx.CurCluster].Layout.ReplicasCount
	if replicaCount <= 1 || nReplicas == 0 {
		log.V(1).Info("Single replica in a shard. Nothing to create a schema from.")
		return nil, nil, nil
	}
	// remove new replica from the list. See https://stackoverflow.com/questions/37334119/how-to-delete-an-element-from-a-slice-in-golang
	log.V(1).Info("Extracting replicated table definitions from %v", "Replicas", replicas)

	systemTables := fmt.Sprintf("remote('%s', system, tables)", strings.Join(replicas, ","))

	sqlDBs := heredoc.Doc(strings.ReplaceAll(`
		SELECT DISTINCT 
			database AS name, 
			concat('CREATE DATABASE IF NOT EXISTS "', name, '"') AS create_db_query
		FROM system.tables
		WHERE database != 'system'
		SETTINGS skip_unavailable_shards = 1`,
		"system.tables", systemTables,
	))
	sqlTables := heredoc.Doc(strings.ReplaceAll(`
		SELECT DISTINCT 
			name, 
			replaceRegexpOne(create_table_query, 'CREATE (TABLE|VIEW|MATERIALIZED VIEW)', 'CREATE \\1 IF NOT EXISTS')
		FROM system.tables
		WHERE database != 'system' and create_table_query != '' and name not like '.inner.%'
		SETTINGS skip_unavailable_shards = 1`,
		"system.tables",
		systemTables,
	))

	var res []error
	names1, sqlStatements1, err1 := s.getObjectListFromClickHouse(CreatePodFQDNsOfInstallation(di), sqlDBs)
	if err1 != nil {
		res = append(res, err1)
	}
	names2, sqlStatements2, err2 := s.getObjectListFromClickHouse(CreatePodFQDNsOfInstallation(di), sqlTables)
	if err2 != nil {
		res = append(res, err2)
	}

	return append(names1, names2...), append(sqlStatements1, sqlStatements2...), errors2.NewAggregate(res)
}

// replicaGetDropTables returns set of 'DROP TABLE ...' SQLs
func (s *Schemer) replicaGetDropTables(namespace string, r *v1.Replica) ([]string, []string, error) {
	// There isn't a separate query for deleting views. To delete a view, use DROP TABLE
	// See https://daisy.yandex/docs/en/query_language/create/
	sql := heredoc.Doc(`
		SELECT
			distinct name, 
			concat('DROP TABLE IF EXISTS "', database, '"."', name, '"') AS drop_db_query
		FROM system.tables
		WHERE engine like 'Replicated%'`,
	)

	names, sqlStatements, _ := s.getObjectListFromClickHouse([]string{CreatePodFQDN(namespace, r)}, sql)
	return names, sqlStatements, nil
}

// DeleteTablesForReplica
func (s *Schemer) DeleteTablesForReplica(namespace string, r *v1.Replica) error {
	log := s.Log
	tableNames, dropTableSQLs, _ := s.replicaGetDropTables(namespace, r)
	log.V(1).Info("Drop tables", "Table", tableNames, "SQL", dropTableSQLs)
	if len(tableNames) > 0 {
		return s.replicaApplySQLs(namespace, r, dropTableSQLs, false)
	}
	return nil
}

// CreateTablesForReplica
func (s *Schemer) CreateTablesForReplica(ctx *memberContext, di *v1.DaisyInstallation, r *v1.Replica) error {
	log := s.Log
	hostname := CreatePodHostname(r)
	log.V(1).Info("Migrating schema objects to replica", "Replica", hostname)

	names, createSQLs, err := s.getCreateReplicaObjects(ctx, di)
	if err != nil {
		return err
	}
	log.V(1).Info("Creating replica objects", "Replica", hostname, "Tables", names, "SQL", createSQLs)
	if err = s.replicaApplySQLs(ctx.Namespace, r, createSQLs, true); err != nil {
		log.Error(err, "Create Replica Objects fail", "SQL", createSQLs)
		return err
	}

	names, createSQLs, err = s.getCreateDistributedObjects(ctx, di, r)
	if err != nil {
		return err
	}
	log.V(1).Info("Creating distributed objects", "Replica", hostname, "Tables", names, "SQL", createSQLs)
	if err := s.replicaApplySQLs(ctx.Namespace, r, createSQLs, true); err != nil {
		log.Error(err, "Create distributed Objects fail", "SQL", createSQLs)
		return err
	}

	return nil
}

// CHIDropDnsCache runs 'DROP DNS CACHE' over the whole CHI
func (s *Schemer) CHIDropDnsCache(chi *v1.DaisyInstallation) error {
	sqls := []string{
		`SYSTEM DROP DNS CACHE`,
	}
	return s.chiApplySQLs(chi, sqls, false)
}

// chiApplySQLs runs set of SQL queries over the whole CHI
func (s *Schemer) chiApplySQLs(di *v1.DaisyInstallation, sqls []string, retry bool) error {
	return s.applySQLs(CreatePodFQDNsOfInstallation(di), sqls, retry)
}

//// clusterApplySQLs runs set of SQL queries over the cluster
//func (s *Schemer) clusterApplySQLs(cluster *chop.ChiCluster, sqls []string, retry bool) error {
//	return s.applySQLs(CreatePodFQDNsOfCluster(cluster), sqls, retry)
//}

// replicaApplySQLs runs set of SQL queries over the replica
func (s *Schemer) replicaApplySQLs(namespace string, r *v1.Replica, sqls []string, retry bool) error {
	replicas := []string{CreatePodFQDN(namespace, r)}
	return s.applySQLs(replicas, sqls, retry)
}

//// shardApplySQLs runs set of SQL queries over the shard replicas
//func (s *Schemer) shardApplySQLs(shard *chop.ChiShard, sqls []string, retry bool) error {
//	return s.applySQLs(CreatePodFQDNsOfShard(shard), sqls, retry)
//}

// applySQLs runs set of SQL queries on set on hosts
// Retry logic traverses the list of SQLs multiple times until all SQLs succeed
func (s *Schemer) applySQLs(hosts []string, sqls []string, retry bool) error {
	log := s.Log
	var err error = nil
	// For each host in the list run all SQL queries
	for _, host := range hosts {
		conn := s.getCHConnection(host)
		maxTries := 1
		if retry {
			maxTries = defaultMaxTries
		}
		err = Retry(maxTries, "Applying sqls", func() error {
			var runErr error = nil
			for i, sql := range sqls {
				if len(sql) == 0 {
					// Skip malformed or already executed SQL query, move to the next one
					continue
				}
				err = conn.Exec(sql)
				if err != nil && strings.Contains(err.Error(), "Code: 253,") && strings.Contains(sql, "CREATE TABLE") {
					log.V(1).Info("Replica is already in ZooKeeper. Trying ATTACH TABLE instead")
					sqlAttach := strings.ReplaceAll(sql, "CREATE TABLE", "ATTACH TABLE")
					err = conn.Exec(sqlAttach)
				}
				if err == nil {
					sqls[i] = "" // Query is executed, removing from the list
				} else {
					runErr = err
				}
			}
			return runErr
		})
	}

	return err
}
