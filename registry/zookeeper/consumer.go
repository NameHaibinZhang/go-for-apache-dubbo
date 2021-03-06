package zookeeper

import (
	"fmt"
)

import (
	log "github.com/AlexStocks/log4go"
	jerrors "github.com/juju/errors"
)

import (
	"github.com/dubbo/go-for-apache-dubbo/plugins"
	"github.com/dubbo/go-for-apache-dubbo/registry"
)

// name: service@protocol
func (r *ZkRegistry) GetService(conf registry.ServiceConfig) ([]registry.ServiceURL, error) {

	var (
		err         error
		dubboPath   string
		nodes       []string
		listener    *zkEventListener
		serviceURL  registry.ServiceURL
		serviceConf registry.ServiceConfig
		ok          bool
	)
	r.listenerLock.Lock()
	listener = r.listener
	r.listenerLock.Unlock()

	if listener != nil {
		listener.listenServiceEvent(conf)
	}

	r.cltLock.Lock()
	serviceConf, ok = r.services[conf.Key()]
	r.cltLock.Unlock()
	if !ok {
		return nil, jerrors.Errorf("Service{%s} has not been registered", conf.Key())
	}
	if !ok {
		return nil, jerrors.Errorf("Service{%s}: failed to get serviceConfigIf type", conf.Key())
	}

	dubboPath = fmt.Sprintf("/dubbo/%s/providers", conf.Service())
	err = r.validateZookeeperClient()
	if err != nil {
		return nil, jerrors.Trace(err)
	}
	r.cltLock.Lock()
	nodes, err = r.client.getChildren(dubboPath)
	r.cltLock.Unlock()
	if err != nil {
		log.Warn("getChildren(dubboPath{%s}) = error{%v}", dubboPath, err)
		return nil, jerrors.Trace(err)
	}

	var listenerServiceMap = make(map[string]registry.ServiceURL)
	for _, n := range nodes {

		serviceURL, err = plugins.DefaultServiceURL()(n)
		if err != nil {
			log.Error("NewDefaultServiceURL({%s}) = error{%v}", n, err)
			continue
		}
		if !serviceConf.ServiceEqual(serviceURL) {
			log.Warn("serviceURL{%s} is not compatible with ServiceConfig{%#v}", serviceURL, serviceConf)
			continue
		}

		_, ok := listenerServiceMap[serviceURL.Query().Get(serviceURL.Location())]
		if !ok {
			listenerServiceMap[serviceURL.Location()] = serviceURL
			continue
		}
	}

	var services []registry.ServiceURL
	for _, service := range listenerServiceMap {
		services = append(services, service)
	}

	return services, nil
}

func (r *ZkRegistry) Subscribe() (registry.Listener, error) {
	r.wg.Add(1)
	return r.getListener()
}

func (r *ZkRegistry) getListener() (*zkEventListener, error) {
	var (
		zkListener *zkEventListener
	)

	r.listenerLock.Lock()
	zkListener = r.listener
	r.listenerLock.Unlock()
	if zkListener != nil {
		return zkListener, nil
	}

	r.cltLock.Lock()
	client := r.client
	r.cltLock.Unlock()
	if client == nil {
		return nil, jerrors.New("zk connection broken")
	}

	// new client & listener
	zkListener = newZkEventListener(r, client)

	r.listenerLock.Lock()
	r.listener = zkListener
	r.listenerLock.Unlock()

	// listen
	r.cltLock.Lock()
	for _, svs := range r.services {
		go zkListener.listenServiceEvent(svs)
	}
	r.cltLock.Unlock()

	return zkListener, nil
}
