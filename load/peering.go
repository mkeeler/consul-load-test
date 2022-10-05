package load

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	petname "github.com/dustinkirkland/golang-petname"
	"github.com/hashicorp/consul/api"
	"github.com/mkeeler/consul-load-test/metrics"
	log "github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

const (
	// acceptorPeerName is the peer name used by the peering clusters
	// the reverse will be <peering address>-<port>
	acceptorPeerName = "acceptor"
)

// KVConfig is the configuration for the KV load generator
type PeeringConfig struct {
	// UpdateRate is the number of KV updates per second
	UpdateRate rate.Limit

	PeeringClusters []string

	NumServices int
}

// peeringCluster represents a peering connection from the
//
//	acceptor to the peer cluster.
type peeringCluster struct {
	// the cli of the peered cluster
	cli      *api.Client
	addr     string
	peerName string
}

func peeringLoad(ctx context.Context, clientAcceptor *api.Client, conf Config, metricsServer *metrics.MetricsServer) <-chan struct{} {
	done := make(chan struct{})

	go func() {
		defer close(done)

		err := testClient(clientAcceptor)
		if err != nil {
			fmt.Printf("STOPPED; error acceptor client: %v\n", err)
			return
		}

		peeringClis, err := getPeeringClients(conf)
		if err != nil {
			log.Errorf("STOPPED; error establishPeering: %v\n", err)
			return
		}
		log.Debugf("Got %d peering Cli", len(peeringClis))

		peeredClusters, err := establishPeerings(ctx, clientAcceptor, peeringClis, conf)
		if err != nil {
			fmt.Printf("STOPPED; error establishPeering: %v\n", err)
			return
		}

		generateLoad(ctx, clientAcceptor, peeredClusters, conf, metricsServer)
		fmt.Println("Finished peering load")
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
	limiter := rate.NewLimiter(conf.UpdateRate, int(conf.UpdateRate*2))
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

			err := limiter.Wait(ctx)
			if err != nil {
				log.Infof("[%d] limiter exit", cid)
				return
			}
			agent := acceptorCli.Agent()
			svcName := petname.Name()
			log.Infof("[%d] ===> registering service %s", cid, svcName)
			err = registerService(ctx, agent, svcName)
			if err != nil {
				log.Warnf("[%d] error registerService %v", cid, err)
				continue
			}
			log.Infof("[%d] service registered", cid)

			// TODO use health API after peer parameter is added.
			// acceptorCli.Health().Service("foo", "", false, &api.QueryOptions{})
			// - get service from the peered cluster: empty expected; otherwise skip
			index, err := getImportedHealthService(ctx, cluster.cli, cluster.addr, svcName, 0)
			if err != nil {
				log.Warnf("[%d] error get service %v", cid, err)
				continue
			}
			log.Infof("[%d] getImportedHealthService consul-index: %d", cid, index)

			waitExportedService := func() <-chan struct{} {
				doneCh := make(chan struct{})

				go func() {
					defer close(doneCh)
					log.Infof("[%d] waiting for exported service %s", cid, svcName)

					// return if the service is exported
					_, err = getImportedHealthService(ctx, cluster.cli, cluster.addr, svcName, int(index))
					if err != nil {
						log.Warnf("[%d] error wait on getting service %v", cid, err)
						return
					}
				}()
				return doneCh
			}
			waitExportServiceCh := waitExportedService()

			t := time.Now()
			err = exportService(context.Background(), acceptorCli, svcName, cluster.peerName)

			if err != nil {
				log.Errorf("[%d] error export service %v", cid, err)
				continue
			}

		work:
			for {
				select {
				case <-time.After(180 * time.Second):
					if metricsServer != nil {
						metricsServer.IncLatencyHistogram(time.Since(t), "peering", "timeout")
					}
					log.Infof("[%d] ===> service is exported: timeout", cid)
					break work
				case <-waitExportServiceCh:
					if metricsServer != nil {
						metricsServer.IncLatencyHistogram(time.Since(t), "peering", "success")
					}
					log.Infof("[%d] ===> service is exported: after %s", cid, time.Since(t))
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

func getImportedHealthService(ctx context.Context, cli *api.Client, addr string, service string, index int) (uint64, error) {
	u := url.URL{
		Scheme: "http",
		Host:   addr,
		Path:   "/v1/health/service/" + service,
	}
	q := u.Query()
	q.Add("peer", acceptorPeerName)

	if index != 0 {
		indexStr := fmt.Sprintf("%d", index)
		q.Add("index", indexStr)
	}

	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		return 0, fmt.Errorf("error get service: %v", err)
	}
	defer resp.Body.Close()

	header := resp.Header

	retIndex := uint64(0)
	if indexStr := header.Get("X-Consul-Index"); indexStr != "" {
		retIndex, err = strconv.ParseUint(indexStr, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("failed to parse X-Consul-Index: %v", err)
		}
	}
	return retIndex, nil
}

func registerService(ctx context.Context, agent *api.Agent, svcName string) error {
	svc := &api.AgentServiceRegistration{
		Name: svcName,
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
							PeerName: peerName,
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
					PeerName: peerName,
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
