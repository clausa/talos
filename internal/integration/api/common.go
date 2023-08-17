// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build integration_api

package api

import (
	"bufio"
	"context"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/siderolabs/talos/internal/integration/base"
	"github.com/siderolabs/talos/pkg/machinery/client"
)

// CommonSuite verifies some default settings such as ulimits.
type CommonSuite struct {
	base.K8sSuite

	ctx       context.Context //nolint:containedctx
	ctxCancel context.CancelFunc
}

// SuiteName ...
func (suite *CommonSuite) SuiteName() string {
	return "api.CommonSuite"
}

// SetupTest ...
func (suite *CommonSuite) SetupTest() {
	if suite.Cluster.Provisioner() == provisionerDocker {
		suite.T().Skip("skipping default values tests in docker")
	}

	// make sure API calls have timeout
	suite.ctx, suite.ctxCancel = context.WithTimeout(context.Background(), 10*time.Minute)
}

// TearDownTest ...
func (suite *CommonSuite) TearDownTest() {
	if suite.ctxCancel != nil {
		suite.ctxCancel()
	}
}

// TestVirtioModulesLoaded verifies that the virtio modules are loaded.
func (suite *CommonSuite) TestVirtioModulesLoaded() {
	if suite.Cluster.Provisioner() == provisionerQEMU {
		suite.T().Skip("skipping virtio modules tests in qemu")
	}

	expectedVirtIOModules := []string{
		"virtio_balloon",
		"virtio_pci",
		"virtio_pci_legacy_dev",
		"virtio_pci_modern_dev",
	}

	node := suite.RandomDiscoveredNodeInternalIP()

	ctx := client.WithNode(suite.ctx, node)

	fileReader, err := suite.Client.Read(ctx, "/proc/modules")
	defer func() {
		err = fileReader.Close()
	}()

	suite.Require().NoError(err)

	scanner := bufio.NewScanner(fileReader)

	var loadedModules []string

	for scanner.Scan() {
		loadedModules = append(loadedModules, strings.Split(scanner.Text(), " ")[0])
	}
	suite.Require().NoError(scanner.Err())

	for _, expectedModule := range expectedVirtIOModules {
		suite.Require().Contains(loadedModules, expectedModule, "expected module %s to be loaded", expectedModule)
	}
}

// TestCommonDefaults verifies that the default ulimits are set.
func (suite *CommonSuite) TestCommonDefaults() {
	expectedUlimit := `
core file size (blocks)         (-c) 0
data seg size (kb)              (-d) unlimited
scheduling priority             (-e) 0
file size (blocks)              (-f) unlimited
max locked memory (kb)          (-l) 8192
max memory size (kb)            (-m) unlimited
open files                      (-n) 1048576
POSIX message queues (bytes)    (-q) 819200
real-time priority              (-r) 0
stack size (kb)                 (-s) 8192
cpu time (seconds)              (-t) unlimited
virtual memory (kb)             (-v) unlimited
file locks                      (-x) unlimited
`

	_, err := suite.Clientset.CoreV1().Pods("default").Create(suite.ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "defaults-test",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "defaults-test",
					Image: "alpine",
					Command: []string{
						"tail",
						"-f",
						"/dev/null",
					},
				},
			},
		},
	}, metav1.CreateOptions{})
	defer suite.Clientset.CoreV1().Pods("default").Delete(suite.ctx, "defaults-test", metav1.DeleteOptions{}) //nolint:errcheck

	suite.Require().NoError(err)

	// wait for the pod to be ready
	suite.Require().NoError(suite.WaitForPodToBeRunning(suite.ctx, 10*time.Minute, "default", "defaults-test"))

	stdout, stderr, err := suite.ExecuteCommandInPod(suite.ctx, "default", "defaults-test", "ulimit -c -d -e -f -l -m -n -q -r -s -t -v -x")
	suite.Require().NoError(err)

	suite.Require().Equal("", stderr)
	suite.Require().Equal(strings.TrimPrefix(expectedUlimit, "\n"), stdout)
}

func init() {
	allSuites = append(allSuites, &CommonSuite{})
}
