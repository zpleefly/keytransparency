// Copyright 2018 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mutationstorage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/keytransparency/core/adminserver"
	"github.com/google/keytransparency/core/integration/storagetest"
	"github.com/google/keytransparency/core/keyserver"
	"github.com/google/keytransparency/impl/sql/testdb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/google/keytransparency/core/api/v1/keytransparency_go_proto"
)

func newForTest(ctx context.Context, t testing.TB, dirID string, logIDs ...int64) (*Mutations, func(context.Context)) {
	db, done := testdb.NewForTest(ctx, t)
	m, err := New(db)
	if err != nil {
		t.Fatalf("Failed to create mutation storage: %v", err)
	}
	if err := m.AddLogs(ctx, dirID, logIDs...); err != nil {
		t.Fatalf("AddLogs(): %v", err)
	}
	return m, done
}

func TestMutationLogsIntegration(t *testing.T) {
	storagetest.RunMutationLogsTests(t,
		func(ctx context.Context, t *testing.T, dirID string, logIDs ...int64) (keyserver.MutationLogs, func(context.Context)) {
			return newForTest(ctx, t, dirID, logIDs...)
		})
}

func TestLogsAdminIntegration(t *testing.T) {
	storagetest.RunLogsAdminTests(t,
		func(ctx context.Context, t *testing.T, dirID string, logIDs ...int64) (adminserver.LogsAdmin, func(context.Context)) {
			return newForTest(ctx, t, dirID, logIDs...)
		})
}

func TestMutationLogsReaderIntegration(t *testing.T) {
	storagetest.RunMutationLogsReaderTests(t,
		func(ctx context.Context, t *testing.T, dirID string, logIDs ...int64) (storagetest.LogsReadWriter, func(context.Context)) {
			return newForTest(ctx, t, dirID, logIDs...)
		})
}

func BenchmarkSend(b *testing.B) {
	ctx := context.Background()
	directoryID := "BenchmarkSend"
	logID := int64(1)
	m, done := newForTest(ctx, b, directoryID, logID)
	defer done(ctx)

	update := &pb.EntryUpdate{Mutation: &pb.SignedEntry{Entry: []byte("xxxxxxxxxxxxxxxxxx")}}
	for _, tc := range []struct {
		batch int
	}{
		{batch: 1},
		{batch: 2},
		{batch: 4},
		{batch: 8},
		{batch: 16},
		{batch: 32},
		{batch: 64},
		{batch: 128},
		{batch: 256},
	} {
		b.Run(fmt.Sprintf("%d", tc.batch), func(b *testing.B) {
			updates := make([]*pb.EntryUpdate, 0, tc.batch)
			for i := 0; i < tc.batch; i++ {
				updates = append(updates, update)
			}
			for n := 0; n < b.N; n++ {
				if _, err := m.Send(ctx, directoryID, logID, updates...); err != nil {
					b.Errorf("Send(): %v", err)
				}
			}
		})
	}
}

func TestSend(t *testing.T) {
	ctx := context.Background()

	directoryID := "TestSend"
	m, done := newForTest(ctx, t, directoryID, 1, 2)
	defer done(ctx)
	update := []byte("bar")
	ts1 := time.Now().Truncate(time.Microsecond)
	ts2 := ts1.Add(1 * time.Microsecond)
	ts3 := ts2.Add(1 * time.Microsecond)

	// Test cases are cumulative. Earlier test caes setup later test cases.
	for _, tc := range []struct {
		desc     string
		ts       time.Time
		wantCode codes.Code
	}{
		// Enforce timestamp uniqueness.
		{desc: "First", ts: ts2},
		{desc: "Second", ts: ts2, wantCode: codes.Aborted},
		// Enforce a monotonically increasing timestamp
		{desc: "Old", ts: ts1, wantCode: codes.Aborted},
		{desc: "New", ts: ts3},
	} {
		err := m.send(ctx, tc.ts, directoryID, 1, update, update)
		if got, want := status.Code(err), tc.wantCode; got != want {
			t.Errorf("%v: send(): %v, got: %v, want %v", tc.desc, err, got, want)
		}
	}
}