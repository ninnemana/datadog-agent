// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package listeners

import (
	"sync"
	"time"

	"github.com/ninnemana/datadog-agent/pkg/autodiscovery/integration"
	"github.com/ninnemana/datadog-agent/pkg/status/health"
	"github.com/ninnemana/datadog-agent/pkg/tagger"
	"github.com/ninnemana/datadog-agent/pkg/util/containers"
	"github.com/ninnemana/datadog-agent/pkg/util/ecs"
	"github.com/ninnemana/datadog-agent/pkg/util/log"
)

// ECSListener implements the ServiceListener interface for fargate-backed ECS cluster.
// It pulls its tasks container list periodically and checks for
// new containers to monitor, and old containers to stop monitoring
type ECSListener struct {
	task       ecs.TaskMetadata
	filter     *containers.Filter
	services   map[string]Service // maps container IDs to services
	newService chan<- Service
	delService chan<- Service
	stop       chan bool
	t          *time.Ticker
	health     *health.Handle
	m          sync.RWMutex
}

// ECSService implements and store results from the Service interface for the ECS listener
type ECSService struct {
	cID           string
	runtime       string
	ADIdentifiers []string
	hosts         map[string]string
	tags          []string
	clusterName   string
	taskFamily    string
	taskVersion   string
	creationTime  integration.CreationTime
}

func init() {
	Register("ecs", NewECSListener)
}

// NewECSListener creates an ECSListener
func NewECSListener() (ServiceListener, error) {
	filter, err := containers.NewFilterFromConfigIncludePause()
	if err != nil {
		return nil, err
	}
	return &ECSListener{
		services: make(map[string]Service),
		stop:     make(chan bool),
		filter:   filter,
		t:        time.NewTicker(2 * time.Second),
		health:   health.Register("ad-ecslistener"),
	}, nil
}

// Listen polls regularly container-related events from the ECS task metadata endpoint and report said containers as Services.
func (l *ECSListener) Listen(newSvc chan<- Service, delSvc chan<- Service) {
	// setup the I/O channels
	l.newService = newSvc
	l.delService = delSvc

	go func() {
		l.refreshServices(true)
		for {
			select {
			case <-l.stop:
				l.health.Deregister()
				return
			case <-l.health.C:
			case <-l.t.C:
				l.refreshServices(false)
			}
		}
	}()
}

// Stop queues a shutdown of ECSListener
func (l *ECSListener) Stop() {
	l.stop <- true
}

// refreshServices queries the task metadata endpoint for fresh info
// compares the container list to the local cache and sends new/dead services
// over newService and delService accordingly
func (l *ECSListener) refreshServices(firstRun bool) {
	meta, err := ecs.GetTaskMetadata()
	if err != nil {
		log.Errorf("failed to get task metadata, not refreshing services - %s", err)
		return
	} else if meta.KnownStatus != "RUNNING" {
		log.Debugf("task %s is not in RUNNING state yet, not refreshing services", meta.Family)
		return
	}
	l.task = meta

	// if not found and running, add it. Else no-op
	// at the end, compare what we saw and what is cached and kill what's not there anymore
	notSeen := make(map[string]interface{})
	for i := range l.services {
		notSeen[i] = nil
	}

	for _, c := range meta.Containers {
		if _, found := l.services[c.DockerID]; found {
			delete(notSeen, c.DockerID)
			continue
		}
		if c.KnownStatus != "RUNNING" {
			log.Debugf("container %s is in status %s - skipping", c.DockerID, c.KnownStatus)
			continue
		}
		if l.filter.IsExcluded(c.DockerName, c.Image) {
			log.Debugf("container %s filtered out: name %q image %q", c.DockerID[:12], c.DockerName, c.Image)
			continue
		}
		s, err := l.createService(c, firstRun)
		if err != nil {
			log.Errorf("couldn't create a service out of container %s - Auto Discovery will ignore it", c.DockerID)
			continue
		}
		l.m.Lock()
		l.services[c.DockerID] = &s
		l.m.Unlock()
		l.newService <- &s
		delete(notSeen, c.DockerID)
	}

	for cID := range notSeen {
		l.m.RLock()
		l.delService <- l.services[cID]
		l.m.RUnlock()
		l.m.Lock()
		delete(l.services, cID)
		l.m.Unlock()
	}
}

func (l *ECSListener) createService(c ecs.Container, firstRun bool) (ECSService, error) {
	var crTime integration.CreationTime
	if firstRun {
		crTime = integration.Before
	} else {
		crTime = integration.After
	}
	svc := ECSService{
		cID:          c.DockerID,
		runtime:      containers.RuntimeNameDocker,
		clusterName:  l.task.ClusterName,
		taskFamily:   l.task.Family,
		taskVersion:  l.task.Version,
		creationTime: crTime,
	}

	// ADIdentifiers
	image := c.Image
	labels := c.Labels
	svc.ADIdentifiers = ComputeContainerServiceIDs(svc.GetEntity(), image, labels)

	// Host
	ips := make(map[string]string)

	for _, net := range c.Networks {
		if net.NetworkMode == "awsvpc" && len(net.IPv4Addresses) > 0 {
			ips["awsvpc"] = string(net.IPv4Addresses[0])
		}
	}
	svc.hosts = ips

	// Tags
	tags, err := tagger.Tag(svc.GetEntity(), tagger.ChecksCardinality)
	if err != nil {
		log.Errorf("Failed to extract tags for container %s - %s", c.DockerID[:12], err)
	}
	svc.tags = tags

	return svc, err
}

// GetEntity returns the unique entity name linked to that service
func (s *ECSService) GetEntity() string {
	return containers.BuildEntityName(s.runtime, s.cID)
}

// GetADIdentifiers returns a set of AD identifiers for a container.
// These id are sorted to reflect the priority we want the ConfigResolver to
// use when matching a template.
//
// When the special identifier label in `identifierLabel` is set by the user,
// it overrides any other meaning of template identification for the service
// and the return value will contain only the label value.
//
// If the special label was not set, the priority order is the following:
//   1. Long image name
//   2. Short image name
func (s *ECSService) GetADIdentifiers() ([]string, error) {
	return s.ADIdentifiers, nil
}

// GetHosts returns the container's hosts
// TODO: using localhost should usually be enough
func (s *ECSService) GetHosts() (map[string]string, error) {
	return s.hosts, nil
}

// GetPorts returns nil and an error because port is not supported in Fargate-based ECS
func (s *ECSService) GetPorts() ([]ContainerPort, error) {
	return nil, ErrNotSupported
}

// GetTags retrieves a container's tags
func (s *ECSService) GetTags() ([]string, error) {
	return s.tags, nil
}

// GetPid inspect the container and return its pid
// TODO: not supported as pid is not in the metadata api
func (s *ECSService) GetPid() (int, error) {
	return -1, ErrNotSupported
}

// GetHostname returns nil and an error because port is not supported in Fargate-based ECS
func (s *ECSService) GetHostname() (string, error) {
	return "", ErrNotSupported
}

// GetCreationTime returns the creation time of the container compare to the agent start.
func (s *ECSService) GetCreationTime() integration.CreationTime {
	return s.creationTime
}
