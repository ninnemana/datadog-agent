// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package pipeline

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/ninnemana/datadog-agent/pkg/status/health"

	"github.com/ninnemana/datadog-agent/pkg/logs/auditor"
	"github.com/ninnemana/datadog-agent/pkg/logs/client"
)

type ProviderTestSuite struct {
	suite.Suite
	p *provider
	a *auditor.Auditor
}

func (suite *ProviderTestSuite) SetupTest() {
	suite.a = auditor.New("", health.Register("fake"))
	suite.p = &provider{
		numberOfPipelines: 3,
		auditor:           suite.a,
		pipelines:         []*Pipeline{},
		endpoints:         client.NewEndpoints(client.Endpoint{}, nil),
	}
}

func (suite *ProviderTestSuite) TestProvider() {
	suite.a.Start()
	suite.p.Start()
	suite.Equal(int32(0), suite.p.currentPipelineIndex)
	suite.Equal(3, len(suite.p.pipelines))

	c := suite.p.NextPipelineChan()
	suite.Equal(int32(1), suite.p.currentPipelineIndex)

	suite.p.NextPipelineChan()
	suite.Equal(int32(2), suite.p.currentPipelineIndex)

	suite.p.NextPipelineChan()
	suite.Equal(int32(0), suite.p.currentPipelineIndex)
	suite.Equal(c, suite.p.NextPipelineChan())
	suite.Equal(int32(1), suite.p.currentPipelineIndex)

	suite.p.Stop()
	suite.a.Stop()
	suite.Nil(suite.p.NextPipelineChan())
}

func TestProviderTestSuite(t *testing.T) {
	suite.Run(t, new(ProviderTestSuite))
}
