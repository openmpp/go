// Copyright (c) 2016 OpenM++
// This code is licensed under the MIT license (see LICENSE.txt for details)

package db

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/openmpp/go/ompp/helper"
)

// Clone return deep copy of source model metadata
func (src *ModelMeta) Clone() (*ModelMeta, error) {

	var dst ModelMeta

	if err := helper.DeepCopy(src, &dst); err != nil {
		return nil, err
	}
	if err := dst.updateInternals(); err != nil {
		return nil, err
	}
	return &dst, nil
}

// Clone return deep copy of source language metadata
func (src *LangMeta) Clone() (*LangMeta, error) {

	dst := &LangMeta{}

	if err := helper.DeepCopy(src, dst); err != nil {
		return nil, err
	}

	// copy language id: it is non-public and update internals
	for k := range src.Lang {
		dst.Lang[k].langId = src.Lang[k].langId
	}
	dst.updateInternals() // update internals

	return dst, nil
}

// FromJson restore model metadata list from json string bytes
func (dst *ModelMeta) FromJson(srcJson []byte) (bool, error) {

	isExist, err := helper.FromJson(srcJson, dst)
	if err != nil {
		return false, err
	}
	if !isExist {
		return false, nil
	}
	dst.updateInternals()
	return true, nil
}

// FromJson restore language list from json string bytes
func (dst *LangMeta) FromJson(srcJson []byte) (bool, error) {

	isExist, err := helper.FromJson(srcJson, dst)
	if err != nil {
		return false, err
	}
	if !isExist {
		return false, nil
	}
	dst.updateInternals()
	return true, nil
}

// IdByCode return language id by language code or first language if code not found
func (langDef *LangMeta) IdByCode(langCode string) (int, bool) {
	if i, ok := langDef.codeIndex[langCode]; ok {
		return langDef.Lang[i].langId, true
	}
	return langDef.Lang[0].langId, false
}

// CodeIdId return language code by language id or first language if id not found
func (langDef *LangMeta) CodeById(langId int) (string, bool) {
	if i, ok := langDef.idIndex[langId]; ok {
		return langDef.Lang[i].LangCode, true
	}
	return langDef.Lang[0].LangCode, false
}

// TypeByKey return index of type by key: typeId
func (meta *ModelMeta) TypeByKey(typeId int) (int, bool) {

	n := len(meta.Type)
	k := sort.Search(n, func(i int) bool {
		return meta.Type[i].TypeId >= typeId
	})
	return k, (k >= 0 && k < n && meta.Type[k].TypeId == typeId)
}

// ParamByKey return index of parameter by key: paramId
func (meta *ModelMeta) ParamByKey(paramId int) (int, bool) {

	n := len(meta.Param)
	k := sort.Search(n, func(i int) bool {
		return meta.Param[i].ParamId >= paramId
	})
	return k, (k >= 0 && k < n && meta.Param[k].ParamId == paramId)
}

// ParamByDigest return index of parameter by parameter digest
func (meta *ModelMeta) ParamByDigest(digest string) (int, bool) {

	for k := range meta.Param {
		if meta.Param[k].Digest == digest {
			return k, true
		}
	}
	return len(meta.Param), false
}

// ParamByName return index of parameter by name
func (meta *ModelMeta) ParamByName(name string) (int, bool) {

	for k := range meta.Param {
		if meta.Param[k].Name == name {
			return k, true
		}
	}
	return len(meta.Param), false
}

// ParamByHid return index of parameter by parameter Hid
func (meta *ModelMeta) ParamByHid(paramHid int) (int, bool) {

	for k := range meta.Param {
		if meta.Param[k].ParamHid == paramHid {
			return k, true
		}
	}
	return len(meta.Param), false
}

// ParamIdByHid return parameter id by Hid or -1 if not found
func (meta *ModelMeta) ParamIdByHid(paramHid int) int {

	if k, ok := meta.ParamByHid(paramHid); ok {
		return meta.Param[k].ParamId
	}
	return -1
}

// ParamHidById return parameter Hid by id or -1 if not found
func (meta *ModelMeta) ParamHidById(paramId int) int {

	if k, ok := meta.ParamByKey(paramId); ok {
		return meta.Param[k].ParamHid
	}
	return -1
}

// DimByKey return index of parameter dimension by key: dimId
func (param *ParamMeta) DimByKey(dimId int) (int, bool) {

	n := len(param.Dim)
	k := sort.Search(n, func(i int) bool {
		return param.Dim[i].DimId >= dimId
	})
	return k, (k >= 0 && k < n && param.Dim[k].DimId == dimId)
}

// OutTableByKey return index of output table by key: tableId
func (meta *ModelMeta) OutTableByKey(tableId int) (int, bool) {

	n := len(meta.Table)
	k := sort.Search(n, func(i int) bool {
		return meta.Table[i].TableId >= tableId
	})
	return k, (k >= 0 && k < n && meta.Table[k].TableId == tableId)
}

// OutTableByName return index of output table by name
func (meta *ModelMeta) OutTableByName(name string) (int, bool) {

	for k := range meta.Table {
		if meta.Table[k].Name == name {
			return k, true
		}
	}
	return len(meta.Table), false
}

// OutTableByDigest return index of output table by digest
func (meta *ModelMeta) OutTableByDigest(digest string) (int, bool) {

	for k := range meta.Table {
		if meta.Table[k].Digest == digest {
			return k, true
		}
	}
	return len(meta.Table), false
}

// OutTableByHid return index of output table by table Hid
func (meta *ModelMeta) OutTableByHid(tableHid int) (int, bool) {

	for k := range meta.Table {
		if meta.Table[k].TableHid == tableHid {
			return k, true
		}
	}
	return len(meta.Table), false
}

// OutTableIdByHid return output table id by Hid or -1 if not found
func (meta *ModelMeta) OutTableIdByHid(tableHid int) int {

	if k, ok := meta.OutTableByHid(tableHid); ok {
		return meta.Table[k].TableId
	}
	return -1
}

// OutTableHidById return output table Hid by id or -1 if not found
func (meta *ModelMeta) OutTableHidById(tableId int) int {

	if k, ok := meta.OutTableByKey(tableId); ok {
		return meta.Table[k].TableHid
	}
	return -1
}

// DimByKey return index of output table dimension by key: dimId
func (table *TableMeta) DimByKey(dimId int) (int, bool) {

	n := len(table.Dim)
	k := sort.Search(n, func(i int) bool {
		return table.Dim[i].DimId >= dimId
	})
	return k, (k >= 0 && k < n && table.Dim[k].DimId == dimId)
}

// IsBool return true if model type is boolean.
func (typeRow *TypeDicRow) IsBool() bool { return strings.ToLower(typeRow.Name) == "bool" }

// IsString return true if model type is string.
func (typeRow *TypeDicRow) IsString() bool { return strings.ToLower(typeRow.Name) == "file" }

// IsFloat return true if model type is float.
func (typeRow *TypeDicRow) IsFloat() bool {
	switch strings.ToLower(typeRow.Name) {
	case "float", "double", "ldouble", "time", "real":
		return true
	}
	return false
}

// IsInt return true if model type is integer (not float, string or boolean).
// If type is not a built-in then it must be integer enums.
func (typeRow *TypeDicRow) IsInt() bool {
	return !typeRow.IsBool() && !typeRow.IsString() && !typeRow.IsFloat()
}

// IsBuiltIn return true if model type is built-in, ie: int, double, logical.
func (typeRow *TypeDicRow) IsBuiltIn() bool { return typeRow.TypeId <= maxBuiltInTypeId }

// sqlColumnType return sql column type, ie: VARCHAR(255)
func (typeRow *TypeDicRow) sqlColumnType(dbFacet Facet) (string, error) {

	// model specific types: it must be enum
	if !typeRow.IsBuiltIn() {
		return "INT", nil
	}

	// built-in types (ordered as in omc grammar for clarity)
	switch strings.ToLower(typeRow.Name) {

	// C++ ambiguous integral type
	// (in C/C++, the signedness of char is not specified)
	case "char":
		return "SMALLINT", nil

	// C++ signed integral types
	case "schar", "short":
		return "SMALLINT", nil

	// C++ signed integral types
	case "int":
		return "INT", nil

	// C++ signed integral types
	case "long", "llong":
		return dbFacet.bigintType(), nil

	// C++ unsigned integral types (including bool)
	case "bool", "uchar":
		return "SMALLINT", nil

	// C++ unsigned integral types (including bool)
	case "ushort":
		return "INT", nil

	// C++ unsigned integral types (including bool)
	case "uint", "ulong", "ullong":
		return dbFacet.bigintType(), nil

	// C++ floating point types
	case "float", "double", "ldouble":
		return dbFacet.floatType(), nil

	// Changeable numeric types
	case "time", "real":
		return dbFacet.floatType(), nil

	// Changeable numeric types
	case "integer", "counter":
		return "INT", nil

	// path to a file (a string)
	case "file":
		return dbFacet.textType(4096), nil
		// go 1.7 max path:
		// linux:  syscall.PathMax
		// win:    syscall.MAX_PATH
	}

	return "", errors.New("invalid type id: " + strconv.Itoa(typeRow.TypeId))
}

// IsRunCompleted retrun true if run status one of: s=success, x=exit, e=error
func IsRunCompleted(status string) bool {
	return status == DoneRunStatus || status == ExitRunStatus || status == ErrorRunStatus
}
