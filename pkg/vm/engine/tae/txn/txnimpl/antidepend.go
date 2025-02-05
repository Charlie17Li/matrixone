// Copyright 2021 Matrix Origin
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package txnimpl

import (
	"fmt"

	"github.com/matrixorigin/matrixone/pkg/common/moerr"
	"github.com/matrixorigin/matrixone/pkg/container/types"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/catalog"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/common"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/iface/txnif"
)

func readWriteConfilictCheck(entry catalog.BaseEntry, ts types.TS) (err error) {
	entry.RLock()
	defer entry.RUnlock()
	needWait, txnToWait := entry.GetLatestNodeLocked().NeedWaitCommitting(ts)
	// TODO:
	// I don't think we need to wait here any more. `block` and `segment` are
	// local metadata and never be involved in a 2PC txn. So a prepared `block`
	// will never be rollbacked
	if needWait {
		entry.RUnlock()
		txnToWait.GetTxnState(true)
		entry.RLock()
	}
	if entry.DeleteBefore(ts) {
		err = moerr.NewTxnRWConflict()
	}
	return
}

type warChecker struct {
	txn         txnif.AsyncTxn
	catalog     *catalog.Catalog
	conflictSet map[common.ID]bool
	readSet     map[common.ID]*catalog.BlockEntry
	cache       map[uint64]*catalog.BlockEntry
}

func newWarChecker(txn txnif.AsyncTxn, c *catalog.Catalog) *warChecker {
	checker := &warChecker{
		txn:         txn,
		catalog:     c,
		conflictSet: make(map[common.ID]bool),
		readSet:     make(map[common.ID]*catalog.BlockEntry),
		cache:       make(map[uint64]*catalog.BlockEntry),
	}
	return checker
}

func (checker *warChecker) CacheGet(
	dbID uint64,
	tableID uint64,
	segmentID uint64,
	blockID uint64) (block *catalog.BlockEntry, err error) {
	block = checker.cacheGet(blockID)
	if block != nil {
		return
	}
	db, err := checker.catalog.GetDatabaseByID(dbID)
	if err != nil {
		return
	}
	table, err := db.GetTableEntryByID(tableID)
	if err != nil {
		return
	}
	segment, err := table.GetSegmentByID(segmentID)
	if err != nil {
		return
	}
	block, err = segment.GetBlockEntryByID(blockID)
	if err != nil {
		return
	}
	checker.Cache(block)
	return
}

func (checker *warChecker) InsertByID(
	dbID uint64,
	tableID uint64,
	segmentID uint64,
	blockID uint64) {
	block, err := checker.CacheGet(dbID, tableID, segmentID, blockID)
	if err != nil {
		panic(err)
	}
	checker.Insert(block)
}

func (checker *warChecker) cacheGet(id uint64) *catalog.BlockEntry {
	return checker.cache[id]
}
func (checker *warChecker) Cache(block *catalog.BlockEntry) {
	checker.cache[block.ID] = block
}

func (checker *warChecker) Insert(block *catalog.BlockEntry) {
	checker.Cache(block)
	id := block.AsCommonID()
	if checker.HasConflict(id) {
		panic(fmt.Sprintf("cannot add conflicted %s into readset", block.String()))
	}
	checker.readSet[*id] = block
}

func (checker *warChecker) checkOne(id *common.ID, ts types.TS) (err error) {
	// defer func() {
	// 	logutil.Infof("checkOne blk=%s ts=%s err=%v", id.BlockString(), ts.ToString(), err)
	// }()
	if checker.HasConflict(id) {
		err = moerr.NewTxnRWConflict()
		return
	}
	entry := checker.readSet[*id]
	if entry == nil {
		return
	}
	return readWriteConfilictCheck(entry.MetaBaseEntry, ts)
}

func (checker *warChecker) checkAll(ts types.TS) (err error) {
	for _, block := range checker.readSet {
		if err = readWriteConfilictCheck(block.MetaBaseEntry, ts); err != nil {
			return
		}
	}
	return
}

func (checker *warChecker) Delete(id *common.ID) {
	checker.conflictSet[*id] = true
	delete(checker.readSet, *id)
}

func (checker *warChecker) HasConflict(id *common.ID) (y bool) {
	_, y = checker.conflictSet[*id]
	return
}
