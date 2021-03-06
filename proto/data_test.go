// Copyright 2014 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License. See the AUTHORS file
// for names of contributors.
//
// Author: Spencer Kimball (spencer.kimball@gmail.com)

package proto

import (
	"bytes"
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"

	gogoproto "github.com/gogo/protobuf/proto"
)

// TestKeyNext tests that the method for creating lexicographic
// successors to byte slices works as expected.
func TestKeyNext(t *testing.T) {
	a := Key("a")
	aNext := a.Next()
	if a.Equal(aNext) {
		t.Errorf("expected key not equal to next")
	}
	if !a.Less(aNext) {
		t.Errorf("expected next key to be greater")
	}

	testCases := []struct {
		key  Key
		next Key
	}{
		{nil, Key("\x00")},
		{Key(""), Key("\x00")},
		{Key("test key"), Key("test key\x00")},
		{Key("\xff"), Key("\xff\x00")},
		{Key("xoxo\x00"), Key("xoxo\x00\x00")},
	}
	for i, c := range testCases {
		if !bytes.Equal(c.key.Next(), c.next) {
			t.Errorf("%d: unexpected next bytes for %q: %q", i, c.key, c.key.Next())
		}
	}
}

func TestKeyPrefixEnd(t *testing.T) {
	a := Key("a1")
	aNext := a.Next()
	aEnd := a.PrefixEnd()
	if !a.Less(aEnd) {
		t.Errorf("expected end key to be greater")
	}
	if !aNext.Less(aEnd) {
		t.Errorf("expected end key to be greater than next")
	}

	testCases := []struct {
		key Key
		end Key
	}{
		{Key{}, KeyMax},
		{Key{0}, Key{0x01}},
		{Key{0xff}, Key{0xff}},
		{Key{0xff, 0xff}, Key{0xff, 0xff}},
		{KeyMax, KeyMax},
		{Key{0xff, 0xfe}, Key{0xff, 0xff}},
		{Key{0x00, 0x00}, Key{0x00, 0x01}},
		{Key{0x00, 0xff}, Key{0x01, 0x00}},
		{Key{0x00, 0xff, 0xff}, Key{0x01, 0x00, 0x00}},
	}
	for i, c := range testCases {
		if !bytes.Equal(c.key.PrefixEnd(), c.end) {
			t.Errorf("%d: unexpected prefix end bytes for %q: %q", i, c.key, c.key.PrefixEnd())
		}
	}
}

func TestKeyEqual(t *testing.T) {
	a1 := Key("a1")
	a2 := Key("a2")
	if !a1.Equal(a1) {
		t.Errorf("expected keys equal")
	}
	if a1.Equal(a2) {
		t.Errorf("expected different keys not equal")
	}
}

func TestKeyLess(t *testing.T) {
	testCases := []struct {
		a, b Key
		less bool
	}{
		{nil, Key("\x00"), true},
		{Key(""), Key("\x00"), true},
		{Key("a"), Key("b"), true},
		{Key("a\x00"), Key("a"), false},
		{Key("a\x00"), Key("a\x01"), true},
	}
	for i, c := range testCases {
		if c.a.Less(c.b) != c.less {
			t.Fatalf("%d: unexpected %q < %q: %t", i, c.a, c.b, c.less)
		}
	}
}

func TestKeyCompare(t *testing.T) {
	testCases := []struct {
		a, b    Key
		compare int
	}{
		{nil, nil, 0},
		{nil, Key("\x00"), -1},
		{Key("\x00"), Key("\x00"), 0},
		{Key(""), Key("\x00"), -1},
		{Key("a"), Key("b"), -1},
		{Key("a\x00"), Key("a"), 1},
		{Key("a\x00"), Key("a\x01"), -1},
	}
	for i, c := range testCases {
		if c.a.Compare(c.b) != c.compare {
			t.Fatalf("%d: unexpected %q.Compare(%q): %d", i, c.a, c.b, c.compare)
		}
	}
}

// TestNextKey tests that the method for creating successors of a Key
// works as expected.
func TestNextKey(t *testing.T) {
	testCases := []struct {
		key  Key
		next Key
	}{
		{nil, Key("\x00")},
		{Key(""), Key("\x00")},
		{Key("test key"), Key("test key\x00")},
		{Key("\xff\xff"), Key("\xff\xff\x00")},
		{Key("xoxo\x00"), Key("xoxo\x00\x00")},
	}
	for i, c := range testCases {
		if !c.key.Next().Equal(c.next) {
			t.Fatalf("%d: unexpected next key for %q: %s", i, c.key, c.key.Next())
		}
	}
}

// TestPrevKey tests that the method for creating the predecessor of a Key
// works as expected.
func TestPrevKey(t *testing.T) {
	testCases := []struct {
		key  Key
		prev Key
	}{
		{Key("\x00"), Key("")},
		{Key("test key\x00"), Key("test key")},
		// "test key\x01" -> "test key\x00\xff..."
		{
			Key("test key\x01"),
			Key(strings.Join([]string{
				"test key\x00",
				strings.Repeat("\xff", KeyMaxLength-9)}, "")),
		},
		// "\x01" -> "\x00\xff..."
		{
			Key("\x01"),
			Key(strings.Join([]string{
				"\x00",
				strings.Repeat("\xff", KeyMaxLength-1)}, "")),
		},
		// "\xff...\x01" -> "\xff...\x00"
		{
			Key(strings.Join([]string{
				strings.Repeat("\xff", KeyMaxLength-1),
				"\x01"}, "")),
			Key(strings.Join([]string{
				strings.Repeat("\xff", KeyMaxLength-1), "\x00"}, "")),
		},
		// "\xff..." -> "\xff...\xfe"
		{
			KeyMax,
			Key(strings.Join([]string{
				strings.Repeat("\xff", KeyMaxLength-1), "\xfe"}, "")),
		},
		// "\xff...\x00" -> "\xff..." with the \x00 removed only
		{
			Key(strings.Join([]string{
				strings.Repeat("\xff", KeyMaxLength-1),
				"\x00"}, "")),
			Key(strings.Repeat("\xff", KeyMaxLength-1)),
		},
	}
	for i, c := range testCases {
		if !c.key.Prev().Equal(c.prev) {
			t.Fatalf("%d: unexpected prev key for %d: %d", i, c.key, c.key.Prev())
		}
	}

	defer func() {
		if err := recover(); err == nil {
			t.Error("Should panic when trying to find prev of keymin")
		}
	}()
	KeyMin.Prev()
}

func TestKeyString(t *testing.T) {
	if KeyMax.String() != "\xff..." {
		t.Errorf("expected key max to display a compact version: %s", KeyMax.String())
	}
	if str := Key(append([]byte("foo"), KeyMax...)).String(); str != "foo\xff..." {
		t.Errorf("expected \"foo\xff...\"; got %q", str)
	}
}

func makeTS(walltime int64, logical int32) Timestamp {
	return Timestamp{
		WallTime: walltime,
		Logical:  logical,
	}
}

func TestLess(t *testing.T) {
	a := Timestamp{}
	b := Timestamp{}
	if a.Less(b) || b.Less(a) {
		t.Errorf("expected %+v == %+v", a, b)
	}
	b = makeTS(1, 0)
	if !a.Less(b) {
		t.Errorf("expected %+v < %+v", a, b)
	}
	a = makeTS(1, 1)
	if !b.Less(a) {
		t.Errorf("expected %+v < %+v", b, a)
	}
}

func TestEqual(t *testing.T) {
	a := Timestamp{}
	b := Timestamp{}
	if !a.Equal(b) {
		t.Errorf("expected %+v == %+v", a, b)
	}
	b = makeTS(1, 0)
	if a.Equal(b) {
		t.Errorf("expected %+v < %+v", a, b)
	}
	a = makeTS(1, 1)
	if b.Equal(a) {
		t.Errorf("expected %+v < %+v", b, a)
	}
}

func TestValueBothBytesAndIntegerSet(t *testing.T) {
	k := []byte("key")
	v := Value{Bytes: []byte("a"), Integer: gogoproto.Int64(0)}
	if err := v.Verify(k); err == nil {
		t.Error("expected error with both byte slice and integer fields set")
	}
}

// TestValueZeroIntegerSerialization verifies that a value with
// integer=0 set can be marshalled and unmarshalled successfully.
// This tests exists because gob serialization treats integers
// and pointers to integers as the same and so loses a proto.Value
// which encodes integer=0.
//
// TODO(spencer): change Value type to switch between integer and
//   []byte value types using a mechanism other than nil pointers.
func TestValueZeroIntegerSerialization(t *testing.T) {
	k := Key("key 00")
	v := Value{Integer: gogoproto.Int64(0)}
	v.InitChecksum(k)

	data, err := gogoproto.Marshal(&v)
	if err != nil {
		t.Fatal(err)
	}
	v2 := &Value{}
	if err = gogoproto.Unmarshal(data, v2); err != nil {
		t.Fatal(err)
	}
	if v2.Integer == nil {
		t.Errorf("expected non-nil integer value; got %s", v2)
	} else if v2.GetInteger() != 0 {
		t.Errorf("expected zero integer value; got %d", v2.GetInteger())
	} else if err = v2.Verify(k); err != nil {
		t.Errorf("failed value verification: %s", err)
	}
}

func TestValueChecksumEmpty(t *testing.T) {
	k := []byte("key")
	v := Value{}
	// Before initializing checksum, always works.
	if err := v.Verify(k); err != nil {
		t.Error(err)
	}
	if err := v.Verify([]byte("key2")); err != nil {
		t.Error(err)
	}
	v.InitChecksum(k)
	if err := v.Verify(k); err != nil {
		t.Error(err)
	}
}

func TestValueChecksumWithBytes(t *testing.T) {
	k := []byte("key")
	v := Value{Bytes: []byte("abc")}
	v.InitChecksum(k)
	if err := v.Verify(k); err != nil {
		t.Error(err)
	}
	// Try a different key; should fail.
	if err := v.Verify([]byte("key2")); err == nil {
		t.Error("expected checksum verification failure on different key")
	}
	// Mess with value.
	v.Bytes = []byte("abcd")
	if err := v.Verify(k); err == nil {
		t.Error("expected checksum verification failure on different value")
	}
}

func TestValueChecksumWithInteger(t *testing.T) {
	k := []byte("key")
	testValues := []int64{0, 1, -1, math.MinInt64, math.MaxInt64}
	for _, i := range testValues {
		v := Value{Integer: gogoproto.Int64(i)}
		v.InitChecksum(k)
		if err := v.Verify(k); err != nil {
			t.Error(err)
		}
		// Try a different key; should fail.
		if err := v.Verify([]byte("key2")); err == nil {
			t.Error("expected checksum verification failure on different key")
		}
		// Mess with value.
		v.Integer = gogoproto.Int64(i + 1)
		if err := v.Verify(k); err == nil {
			t.Error("expected checksum verification failure on different value")
		}
	}
}

func TestGCMetadataEstimatedBytes(t *testing.T) {
	gc := GCMetadata{
		LastGCNanos: 0,
		TTLSeconds:  100,
		ByteCounts:  []int64{10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
	}
	for i := int64(-1); i < int64(110); i++ {
		expBytes := i / 10
		if eb := gc.EstimatedBytes(time.Unix(i, 0), 0); eb != expBytes {
			t.Errorf("expected %d @%ds; got %d", expBytes, i, eb)
		}
	}

	// Throw in an estimation of current non-live bytes. These get
	// prorated based on how far the elapsed time has exceeded
	// TTLSeconds. We scale here from elapsed time being equal to
	// TTL to elapsed time being 10x TTL. at 1x TTL, we get 0 prorated
	// bytes from current non-live count. at 10x TTL, we get 90%.
	for i := int64(100); i < int64(1000); i += 100 {
		fraction := (float64(i)/100 - 1) / (float64(i) / 100)
		expBytes := 10 + int64(100*fraction)
		if eb := gc.EstimatedBytes(time.Unix(i, 0), 110); eb != expBytes {
			t.Errorf("expected %d @%ds; got %d", expBytes, i, eb)
		}
	}
}

// TestNodeList verifies that its public methods Add() and Contain()
// operate as expected.
func TestNodeList(t *testing.T) {
	sn := NodeList{}
	items := append([]int{109, 104, 102, 108, 1000}, rand.Perm(100)...)
	for i, _ := range items {
		n := int32(items[i])
		if sn.Contains(n) {
			t.Fatalf("%d: false positive hit for %d on slice %v",
				i, n, sn.GetNodes())
		}
		// Add this item and, for good measure, all the previous ones.
		for j := i; j >= 0; j-- {
			sn.Add(int32(items[j]))
		}
		if nodes := sn.GetNodes(); len(nodes) != i+1 {
			t.Fatalf("%d: missing values or duplicates: %v",
				i, nodes)
		}
		if !sn.Contains(n) {
			t.Fatalf("%d: false negative hit for %d on slice %v",
				i, n, sn.GetNodes())
		}
	}
}
