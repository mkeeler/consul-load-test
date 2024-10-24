package peering

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	petname "github.com/dustinkirkland/golang-petname"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/mkeeler/consul-load-test/load/config"
	"golang.org/x/time/rate"
)

type LoadGenerator struct {
	config.GeneratorConfig
	conf UserConfig
}

// peeringCluster represents a peering connection from the acceptor to the peer cluster.
type peeringCluster struct {
	// the cli of the peered cluster
	cli      *api.Client
	addr     string
	peerName string
}

func NewLoadGenerator(conf Config) *LoadGenerator {
	return &LoadGenerator{
		GeneratorConfig: conf.GeneratorConfig,
		conf:            conf.UserConfig,
	}
}

func (lg *LoadGenerator) Initialize(ctx context.Context) error {
	return nil
}

func (lg *LoadGenerator) Run(ctx context.Context) error {
	lg.Logger.Info("starting peering load")
	defer lg.Logger.Info("finished peering load")

	err := testClient(lg.Client)
	if err != nil {
		lg.Logger.Error("error creating test client", "error", err)
		return err
	}

	peeringClis, err := lg.getPeeringClients()
	if err != nil {
		lg.Logger.Error("error getting peering clients", "error", err)
		return err
	}
	lg.Logger.Debug("peering clients created", "count", len(peeringClis))

	peeredClusters, err := lg.establishPeerings(ctx, peeringClis)
	if err != nil {
		lg.Logger.Error("error establishing peerings", "error", err)
		return err
	}

	return lg.generateLoad(ctx, peeredClusters)
}

func (lg *LoadGenerator) generateLoad(ctx context.Context, peerClusters []*peeringCluster) error {
	workerDoneList := []<-chan struct{}{}
	var mu sync.Mutex

	for i, pc := range peerClusters {
		cid := i

		lg.Logger.Info("Start load", "id", cid, "cluster", pc.addr)

		c := lg.doLoad(ctx, cid, pc)

		mu.Lock()
		workerDoneList = append(workerDoneList, c)
		mu.Unlock()
	}

	// wait for all worker to exit
	waitForChannelsToClose(ctx, workerDoneList...)
	return nil
}

func (lg *LoadGenerator) doLoad(ctx context.Context, cid int, cluster *peeringCluster) <-chan struct{} {
	logger := lg.Logger.With("id", cid, "cluster", cluster.addr)
	ctx = hclog.WithContext(ctx, logger)
	limiter := rate.NewLimiter(lg.conf.RegisterLimit, int(lg.conf.NumServices))
	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				logger.Info("doLoad exit by ctx.Done()", "id", cid)
				return
			default:
			}

			if err := limiter.Wait(ctx); err != nil {
				logger.Info("[%d] limiter exit", cid)
				return
			}
			agent := lg.Client.Agent()
			svcName := petname.Name()

			logger.Info("registering service", "service", svcName)
			if err := registerService(agent, svcName); err != nil {
				logger.Warn("error registering service", "error", err)
				continue
			}
			logger.Info("service registered", "service", svcName)

			// Get the index from health endpoint: non-blocking query
			_, meta, err := cluster.cli.Health().Service(svcName, "", false, &api.QueryOptions{
				Peer: acceptorPeerName,
			})
			if err != nil {
				logger.Warn("error get service health", "service", svcName, "error", err)
				continue
			}
			index := meta.LastIndex
			logger.Info("health service consul-index", "service", svcName, "last-index", meta.LastIndex)

			waitExportedService := func() <-chan struct{} {
				doneCh := make(chan struct{})

				go func() {
					defer close(doneCh)
					logger.Info("waiting for exported service", "service", svcName)

					// Use the index from the health endpoint response above to
					// issue a blocking query; this will block until the service
					// has been exported to the peer.
					_, _, err := cluster.cli.Health().Service(svcName, "", false, &api.QueryOptions{
						Peer:      acceptorPeerName,
						WaitIndex: index,
					})
					if err != nil {
						logger.Warn("error wait on service health", "service", svcName, "error", err)
						return
					}
				}()
				return doneCh
			}
			waitExportServiceCh := waitExportedService()

			t := time.Now()

			if err := exportService(context.Background(), lg.Client, svcName, cluster.peerName); err != nil {
				logger.Error("error export service", "error", err)
				continue
			}

		work:
			for {
				select {
				case <-time.After(lg.conf.ExportTimeout):
					if lg.MetricsServer != nil {
						lg.MetricsServer.IncLatencyHistogram(time.Since(t), "peering", "timeout")
					}
					logger.Info("service is exported (timeout)", "service", svcName)
					break work
				case <-waitExportServiceCh:
					if lg.MetricsServer != nil {
						lg.MetricsServer.IncLatencyHistogram(time.Since(t), "peering", "success")
					}
					logger.Info("service is exported", "service", svcName, "after", time.Since(t))
					break work
				case <-ctx.Done():
					logger.Info("doLoad exit by ctx.Done()!")
					return
				}
			}
		}
	}()

	return done
}

func registerService(agent *api.Agent, svcName string) error {
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
	logger := hclog.FromContext(ctx)
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
	logger.Info("Exported config entry updated:", updated)

	return nil
}

func (lg *LoadGenerator) getPeeringClients() ([]*api.Client, error) {
	peeringClients := []*api.Client{}
	var err error

	for _, addr := range lg.conf.PeeringClusters {
		lg.Logger.Debug("Creating cli for peering", "addr", addr)
		cfg := api.DefaultConfig()
		cfg.Address = addr
		var cli *api.Client
		cli, err = api.NewClient(cfg)
		if err != nil {
			lg.Logger.Error("error new client")
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

func (lg *LoadGenerator) establishPeerings(ctx context.Context, peeringClis []*api.Client) ([]*peeringCluster, error) {
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
			peerName := strings.ReplaceAll(lg.conf.PeeringClusters[cid], ":", "-")
			peerName = strings.ReplaceAll(peerName, ".", "-")
			err := peerClusters(ctx, lg.Client, peeringCli, peerName)
			if err != nil {
				lg.Logger.Error("error peering with cluster", "cluster", lg.conf.PeeringClusters[cid], "error", err)
				return
			}

			pc := &peeringCluster{
				cli:      peeringCli,
				addr:     lg.conf.PeeringClusters[cid],
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

	hclog.FromContext(ctx).Debug("Generated token response", "token", resp.PeeringToken)

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

func waitForChannelsToClose(ctx context.Context, chans ...<-chan struct{}) {
	logger := hclog.FromContext(ctx)
	t := time.Now()
	logger.Info("waiting for all workers...")
	for _, v := range chans {
		<-v
		logger.Info("chan closed", "duration", time.Since(t))
	}
	logger.Info("all chans closed", "duration", time.Since(t))
}
