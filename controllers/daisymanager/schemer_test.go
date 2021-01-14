package daisymanager

import (
	v1 "github.com/daisy/daisy-operator/api/v1"
	testing2 "github.com/go-logr/logr/testing"
	"testing"

	"github.com/go-logr/logr"

	"github.com/daisy/daisy-operator/pkg/daisy"
)

func TestSchemer_ReplicaCreateTables(t *testing.T) {
	log := testing2.TestLogger{T: t}
	type fields struct {
		Username string
		Password string
		Port     int
		Pool     daisy.ConnectionPool
		Log      logr.Logger
	}
	type args struct {
		ctx    *memberContext
		di     *v1.DaisyInstallation
		status *v1.DaisyInstallationStatus
		r      *v1.Replica
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "no distributed table",
			args: args{
				ctx: &memberContext{
					CurCluster: "cluster",
					CurShard:   "test-installation-cluster-0",
					CurReplica: "test-installation-cluster-0-0",
				},
				di: newTestInstallation(2, 2),
				status: &v1.DaisyInstallationStatus{
					Clusters: map[string]v1.ClusterStatus{
						"cluster": {
							Shards: map[string]v1.ShardStatus{
								"test-installation-cluster-0": {
									Replicas: map[string]v1.ReplicaStatus{
										"test-installation-cluster-0-0": {
											Phase: v1.ReadyPhase,
										},
									},
								},
							},
						},
					},
				},
				r: &v1.Replica{
					Name: "test-installation-cluster-0-0",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			di := tt.args.di
			fillLayout("cluster", di)
			di.Status = *tt.args.status
			s, conn := NewFakeSchemer("", "", 0, log)
			//conn.AddQueryResult("", )
			if err := s.CreateTablesForReplica(tt.args.ctx, tt.args.di, tt.args.r); (err != nil) != tt.wantErr {
				t.Errorf("CreateTablesForReplica() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(conn.Tracker) <= 0 {
				t.Errorf("No sql executed")
			}
		})
	}
}

func NewFakeSchemer(username, password string, port int, logger logr.Logger) (*Schemer, *daisy.FakeConnection) {
	fakeConnection := daisy.NewFakeConnection()
	return &Schemer{
		Username: username,
		Password: password,
		Port:     port,
		Log:      logger.WithName("schemer"),
		Pool: &daisy.FakeConnectionPool{
			Conn: fakeConnection,
		},
	}, fakeConnection
}
