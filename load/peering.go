package load

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	petname "github.com/dustinkirkland/golang-petname"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/mkeeler/consul-load-test/metrics"
	log "github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

const (
	// acceptorPeerName is the peer name used by the peering clusters
	// the reverse will be <peering address>-<port>
	acceptorPeerName = "acceptor"
)

var defaultExportTimeout = time.Duration(180 * time.Second)

// PeeringConfig is the configuration for the Peering load generator
type PeeringConfig struct {
	// RegisterLimit is the time in seconds to wait between registering services
	RegisterLimit rate.Limit
	// ExportTimeout is the maximum time to wait for services to become exported to a remote peer
	ExportTimeout time.Duration
	// PeeringClusters holds the addresses for the servers that will be peered
	PeeringClusters []string
	// NumServices is the number of services to register and export during the load test
	NumServices int
}

// Normalize validates the Peering load test configuration. RegisterLimit and
// NumServices must not be zero, otherwise, an error is returned.
func (c *PeeringConfig) Normalize() error {
	if c.RegisterLimit == 0.0 {
		return fmt.Errorf("invalid RegisterLimit configuration: %v", c.RegisterLimit)
	}
	if c.NumServices == 0 {
		return fmt.Errorf("invalid NumServices configuration: %v", c.NumServices)
	}
	if c.ExportTimeout == 0 {
		c.ExportTimeout = defaultExportTimeout
	}
	return nil
}

// peeringCluster represents a peering connection from the acceptor to the peer cluster.
type peeringCluster struct {
	// the cli of the peered cluster
	cli      *api.Client
	addr     string
	peerName string
}

func peeringLoad(ctx context.Context, clientAcceptor *api.Client, conf Config, metricsServer *metrics.MetricsServer) <-chan struct{} {
	logger := hclog.FromContext(ctx).Named("peering")
	ctx = hclog.WithContext(ctx, logger)

	done := make(chan struct{})

	go func() {
		logger.Info("Starting peering load")
		defer logger.Info("Stopped peering load")
		defer close(done)

		err := testClient(clientAcceptor)
		if err != nil {
			logger.Error("error creating test client", "error", err)
			return
		}

		peeringClis, err := getPeeringClients(conf)
		if err != nil {
			logger.Error("error getting peering clients", "error", err)
			return
		}
		logger.Debug("peering clients created", "count", len(peeringClis))

		peeredClusters, err := establishPeerings(ctx, clientAcceptor, peeringClis, conf)
		if err != nil {
			log.Error("error establishing peerings", "error", err)
			return
		}

		generateLoad(ctx, clientAcceptor, peeredClusters, conf, metricsServer)
	}()
	return done
}

func generateLoad(ctx context.Context, acceptorCli *api.Client, peerClusters []*peeringCluster, conf Config, metricsServer *metrics.MetricsServer) error {
	workerDoneList := []<-chan struct{}{}
	var mu sync.Mutex

	for i, pc := range peerClusters {
		cid := i

		log.Infof("Start load id %d for %s", cid, pc.addr)
		c := doLoad(ctx, cid, acceptorCli, pc, *conf.Peering, metricsServer)

		mu.Lock()
		workerDoneList = append(workerDoneList, c)
		mu.Unlock()
	}

	// wait for all worker to exit
	waitForChannelsToClose(workerDoneList...)
	return nil
}

func doLoad(ctx context.Context, cid int, acceptorCli *api.Client, cluster *peeringCluster, conf PeeringConfig, metricsServer *metrics.MetricsServer) <-chan struct{} {
	limiter := rate.NewLimiter(conf.RegisterLimit, int(conf.NumServices))
	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				log.Infof("[%d] doLoad exit by ctx.Done()!", cid)
				return
			default:
			}

			if err := limiter.Wait(ctx); err != nil {
				log.Infof("[%d] limiter exit", cid)
				return
			}
			agent := acceptorCli.Agent()
			svcName := petname.Name()

			log.Infof("[%d] ===> registering service %s", cid, svcName)
			if err := registerService(ctx, agent, svcName); err != nil {
				log.Warnf("[%d] error registerService %v", cid, err)
				continue
			}
			log.Infof("[%d] service registered", cid)

			// Get the index from health endpoint: non-blocking query
			_, meta, err := cluster.cli.Health().Service(svcName, "", false, &api.QueryOptions{
				Peer: acceptorPeerName,
			})
			if err != nil {
				log.Warnf("[%d] error get service health %s: %v", cid, svcName, err)
				continue
			}
			index := meta.LastIndex
			log.Infof("[%d] health service consul-index: %s, %d", cid, svcName, meta.LastIndex)

			waitExportedService := func() <-chan struct{} {
				doneCh := make(chan struct{})

				go func() {
					defer close(doneCh)
					log.Infof("[%d] waiting for exported service %s", cid, svcName)

					// Use the index from the health endpoint response above to
					// issue a blocking query; this will block until the service
					// has been exported to the peer.
					_, _, err := cluster.cli.Health().Service(svcName, "", false, &api.QueryOptions{
						Peer:      acceptorPeerName,
						WaitIndex: index,
					})
					if err != nil {
						log.Warnf("[%d] error wait on service health %s: %v", cid, svcName, err)
						return
					}
				}()
				return doneCh
			}
			waitExportServiceCh := waitExportedService()

			t := time.Now()

			if err := exportService(context.Background(), acceptorCli, svcName, cluster.peerName); err != nil {
				log.Errorf("[%d] error export service %v", cid, err)
				continue
			}

		work:
			for {
				select {
				case <-time.After(conf.ExportTimeout):
					if metricsServer != nil {
						metricsServer.IncLatencyHistogram(time.Since(t), "peering", "timeout")
					}
					log.Infof("[%d] ===> service is exported: %s (timeout)", cid, svcName)
					break work
				case <-waitExportServiceCh:
					if metricsServer != nil {
						metricsServer.IncLatencyHistogram(time.Since(t), "peering", "success")
					}
					log.Infof("[%d] ===> service is exported: %s after %s", cid, svcName, time.Since(t))
					break work
				case <-ctx.Done():
					log.Infof("[%d] ===> doLoad exit by ctx.Done()!", cid)
					return
				}
			}
		}
	}()

	return done
}

func registerService(ctx context.Context, agent *api.Agent, svcName string) error {
	timestamp := fmt.Sprintf("time: %v", time.Now().UnixMilli())
	svc := &api.AgentServiceRegistration{
		Name: svcName,
		Tags: []string{timestamp},
	}
	err := agent.ServiceRegister(svc)
	if err != nil {
		return fmt.Errorf("error agent service register: %v", err)
	}
	return nil
}

func exportService(ctx context.Context, cli *api.Client, svcName string, peerName string) error {
	kind := "exported-services"
	configEntries := cli.ConfigEntries()

	configEntry, _, err := configEntries.Get(kind, "default", &api.QueryOptions{})
	if err != nil {
		if !strings.Contains(err.Error(), "Config entry not found") {
			return fmt.Errorf("error get config entries: %v", err)
		}
	}

	var exConfigEntry *api.ExportedServicesConfigEntry
	if configEntry == nil {
		exConfigEntry = &api.ExportedServicesConfigEntry{
			Name: "default",
			Services: []api.ExportedService{
				{
					Name: svcName,
					Consumers: []api.ServiceConsumer{
						{
							Peer: peerName,
						},
					},
				},
			},
		}
	} else {
		exConfigEntry = configEntry.(*api.ExportedServicesConfigEntry)
		exConfigEntry.Services = append(exConfigEntry.Services, api.ExportedService{
			Name: svcName,
			Consumers: []api.ServiceConsumer{
				{
					Peer: peerName,
				},
			},
		})
	}

	updated, _, err := configEntries.Set(exConfigEntry, &api.WriteOptions{})
	if err != nil {
		return fmt.Errorf("error write config entries: %v", err)
	}
	log.Debugln("Exported config entry updated:", updated)

	return nil
}

func getPeeringClients(conf Config) ([]*api.Client, error) {
	peeringClients := []*api.Client{}
	var err error

	for _, addr := range conf.Peering.PeeringClusters {
		log.Println("Creating cli for peering addr:", addr)
		cfg := api.DefaultConfig()
		cfg.Address = addr
		var cli *api.Client
		cli, err = api.NewClient(cfg)
		if err != nil {
			log.Errorln("error new client")
			break
		}
		err = testClient(cli)
		if err != nil {
			break
		}
		peeringClients = append(peeringClients, cli)
	}
	if err != nil {
		return nil, fmt.Errorf("err %v", err)
	}
	return peeringClients, err
}

func establishPeerings(ctx context.Context, acceptorCli *api.Client, peeringClis []*api.Client, conf Config) ([]*peeringCluster, error) {
	peeringClusters := []*peeringCluster{}
	var mu sync.Mutex

	// create some workers to establish the peering
	var wg sync.WaitGroup
	for i, cli := range peeringClis {
		wg.Add(1)
		cid := i
		peeringCli := cli

		go func() {
			defer wg.Done()
			peerName := strings.ReplaceAll(conf.Peering.PeeringClusters[cid], ":", "-")
			peerName = strings.ReplaceAll(peerName, ".", "-")
			err := peerClusters(ctx, acceptorCli, peeringCli, peerName)
			if err != nil {
				log.Errorf("error peering with cluster %v: %v", conf.Peering.PeeringClusters[cid], err)
				return
			}

			pc := &peeringCluster{
				cli:      peeringCli,
				addr:     conf.Peering.PeeringClusters[cid],
				peerName: peerName,
			}
			mu.Lock()
			defer mu.Unlock()
			peeringClusters = append(peeringClusters, pc)
		}()
	}
	wg.Wait()
	return peeringClusters, nil
}

func peerClusters(ctx context.Context, acceptorCli *api.Client, peeringCli *api.Client, peerName string) error {
	var err error

	peering := acceptorCli.Peerings()

	req := api.PeeringGenerateTokenRequest{
		PeerName: peerName,
	}
	resp, _, err := peering.GenerateToken(ctx, req, &api.WriteOptions{})
	if err != nil {
		return fmt.Errorf("error generating token: %v", err)
	}
	log.Debugln("Generated token response", resp.PeeringToken)

	p := peeringCli.Peerings()
	establishReq := api.PeeringEstablishRequest{
		PeerName:     acceptorPeerName,
		PeeringToken: resp.PeeringToken,
	}
	_, _, err = p.Establish(ctx, establishReq, &api.WriteOptions{})
	if err != nil {
		return fmt.Errorf("error establish peering: %v", err)
	}
	return err
}

func testClient(client *api.Client) error {

	pair := api.KVPair{
		Key:   "foo",
		Value: []byte("bar"),
	}

	_, err := client.KV().Put(&pair, nil)
	return err
}

func waitForChannelsToClose(chans ...<-chan struct{}) {
	t := time.Now()
	log.Infof("waiting for all workers...")
	for _, v := range chans {
		<-v
		log.Infof("%v for chan to close\n", time.Since(t))
	}
	log.Infof("%v for channels to close\n", time.Since(t))
}
