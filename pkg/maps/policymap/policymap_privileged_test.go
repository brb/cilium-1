// SPDX-License-Identifier: Apache-2.0
// Copyright 2018-2020 Authors of Cilium

//go:build privileged_tests
// +build privileged_tests

package policymap

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/unix"
	. "gopkg.in/check.v1"

	"github.com/cilium/ebpf/rlimit"

	"github.com/cilium/cilium/pkg/bpf"
	"github.com/cilium/cilium/pkg/checker"
	"github.com/cilium/cilium/pkg/logging"
	"github.com/cilium/cilium/pkg/logging/logfields"
	"github.com/cilium/cilium/pkg/policy/trafficdirection"
	"github.com/cilium/cilium/pkg/u8proto"
)

var log = logging.DefaultLogger.WithField(logfields.LogSubsys, "map-policy")

func Test(t *testing.T) {
	TestingT(t)
}

type PolicyMapTestSuite struct{}

var (
	_ = Suite(&PolicyMapTestSuite{})

	testMap = newMap("cilium_policy_test")
)

func runTests(m *testing.M) (int, error) {
	bpf.CheckOrMountFS("")
	if err := rlimit.RemoveMemlock(); err != nil {
		return 1, fmt.Errorf("Failed to configure rlimit")
	}

	_ = os.RemoveAll(bpf.MapPath("cilium_policy_test"))
	_, err := testMap.OpenOrCreate()
	if err != nil {
		return 1, fmt.Errorf("Failed to create map")
	}
	defer func() {
		path, _ := testMap.Path()
		os.Remove(path)
	}()
	defer testMap.Close()

	return m.Run(), nil
}

func TestMain(m *testing.M) {
	exitCode, err := runTests(m)
	if err != nil {
		log.Fatal(err)
	}
	os.Exit(exitCode)
}

func (pm *PolicyMapTestSuite) TearDownTest(c *C) {
	testMap.DeleteAll()
}

func (pm *PolicyMapTestSuite) TestPolicyMapDumpToSlice(c *C) {
	c.Assert(testMap, NotNil)

	fooEntry := newKey(1, 1, 1, 1)
	err := testMap.AllowKey(fooEntry, 0)
	c.Assert(err, IsNil)

	dump, err := testMap.DumpToSlice()
	c.Assert(err, IsNil)
	c.Assert(len(dump), Equals, 1)

	// FIXME: It's weird that AllowKey() does the implicit byteorder
	//        conversion above. But not really a bug, so work around it.
	fooEntry = fooEntry.ToNetwork()
	c.Assert(dump[0].Key, checker.DeepEquals, fooEntry)

	// Special case: allow-all entry
	barEntry := newKey(0, 0, 0, 0)
	err = testMap.AllowKey(barEntry, 0)
	c.Assert(err, IsNil)

	dump, err = testMap.DumpToSlice()
	c.Assert(err, IsNil)
	c.Assert(len(dump), Equals, 2)
}

func (pm *PolicyMapTestSuite) TestDeleteNonexistentKey(c *C) {
	key := newKey(27, 80, u8proto.ANY, trafficdirection.Ingress)
	err := testMap.Map.Delete(&key)
	c.Assert(err, Not(IsNil))
	var errno unix.Errno
	c.Assert(errors.As(err, &errno), Equals, true)
	c.Assert(errno, Equals, unix.ENOENT)
}

func (pm *PolicyMapTestSuite) TestDenyPolicyMapDumpToSlice(c *C) {
	c.Assert(testMap, NotNil)

	fooEntry := newKey(1, 1, 1, 1)
	fooValue := newEntry(0, NewPolicyEntryFlag(&PolicyEntryFlagParam{IsDeny: true}))
	err := testMap.DenyKey(fooEntry)
	c.Assert(err, IsNil)

	dump, err := testMap.DumpToSlice()
	c.Assert(err, IsNil)
	c.Assert(len(dump), Equals, 1)

	// FIXME: It's weird that AllowKey() does the implicit byteorder
	//        conversion above. But not really a bug, so work around it.
	fooEntry = fooEntry.ToNetwork()
	c.Assert(dump[0].Key, checker.DeepEquals, fooEntry)
	c.Assert(dump[0].PolicyEntry, checker.DeepEquals, fooValue)

	// Special case: deny-all entry
	barEntry := newKey(0, 0, 0, 0)
	err = testMap.DenyKey(barEntry)
	c.Assert(err, IsNil)

	dump, err = testMap.DumpToSlice()
	c.Assert(err, IsNil)
	c.Assert(len(dump), Equals, 2)
}
