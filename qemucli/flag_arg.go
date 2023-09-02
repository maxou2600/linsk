// Linsk - A utility to access Linux-native file systems on non-Linux operating systems.
// Copyright (c) 2023 The Linsk Authors.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package qemucli

import (
	"github.com/pkg/errors"
)

type FlagArg struct {
	key string
}

func MustNewFlagArg(key string) *FlagArg {
	a, err := NewFlagArg(key)
	if err != nil {
		panic(err)
	}

	return a
}

func NewFlagArg(key string) (*FlagArg, error) {
	a := &FlagArg{
		key: key,
	}

	// Preflight arg key/type check.
	err := validateArgKey(a.key, a.ValueType())
	if err != nil {
		return nil, errors.Wrap(err, "validate arg key")
	}

	return a, nil
}

func (a *FlagArg) StringKey() string {
	return a.key
}

func (a *FlagArg) StringValue() string {
	// Boolean flags have no value.
	return ""
}

func (a *FlagArg) ValueType() ArgAcceptedValue {
	return ArgAcceptedValueNone
}
