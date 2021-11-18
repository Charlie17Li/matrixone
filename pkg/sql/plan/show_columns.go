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

package plan

import (
	"matrixone/pkg/errno"
	"matrixone/pkg/sql/errors"
	"matrixone/pkg/sql/parsers/tree"
)

func (b *build) BuildShowColumns(stmt *tree.ShowColumns, plan *ShowColumns) error {
	tbl := stmt.Table.ToTableName()
	_, _, r, err := b.tableName(&tbl)
	if err != nil {
		return err
	}
	if stmt.Where != nil {
		return errors.New(errno.FeatureNotSupported, "not support where for show columns")
	}
	if stmt.Like != nil {
		plan.Like = []byte(stmt.Like.Right.String())
	}
	plan.Relation = r
	return nil
}