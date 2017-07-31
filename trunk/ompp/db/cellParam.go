// Copyright (c) 2016 OpenM++
// This code is licensed under the MIT license (see LICENSE.txt for details)

package db

import (
	"errors"
	"fmt"
	"strconv"
)

// CellParam is value of input parameter.
type CellParam struct {
	cellValue     // dimensions and value
	SubId     int // parameter subvalue id
}

// CsvFileName return file name of csv file to store parameter rows
func (CellParam) CsvFileName(modelDef *ModelMeta, name string) (string, error) {

	// validate parameters
	if modelDef == nil {
		return "", errors.New("invalid (empty) model metadata, look like model not found")
	}
	if name == "" {
		return "", errors.New("invalid (empty) output table name")
	}

	// find parameter by name
	k, ok := modelDef.ParamByName(name)
	if !ok {
		return "", errors.New("parameter not found: " + name)
	}

	return modelDef.Param[k].Name + ".csv", nil
}

// CsvHeader retrun first line for csv file: column names, it's look like: sub_id,dim0,dim1,param_value.
func (CellParam) CsvHeader(modelDef *ModelMeta, name string, isIdHeader bool, valueName string) ([]string, error) {

	// validate parameters
	if modelDef == nil {
		return nil, errors.New("invalid (empty) model metadata, look like model not found")
	}
	if name == "" {
		return nil, errors.New("invalid (empty) parameter name")
	}

	// find parameter by name
	k, ok := modelDef.ParamByName(name)
	if !ok {
		return nil, errors.New("parameter not found: " + name)
	}
	param := &modelDef.Param[k]

	// make first line columns
	h := make([]string, param.Rank+2)

	h[0] = "sub_id"
	for k := range param.Dim {
		h[k+1] = param.Dim[k].Name
	}
	h[param.Rank+1] = "param_value"

	return h, nil
}

// CsvToIdRow return converter from parameter cell (sub id, dimensions, value) to csv row []string.
//
// Converter simply does Sprint() for each sub-value id, dimension item id and value.
// Converter will retrun error if len(row) not equal to number of fields in csv record.
// Double format string is used if parameter type is float, double, long double
func (CellParam) CsvToIdRow(
	modelDef *ModelMeta, name string, doubleFmt string, valueName string) (
	func(interface{}, []string) error, error) {

	// validate parameters
	if modelDef == nil {
		return nil, errors.New("invalid (empty) model metadata, look like model not found")
	}
	if name == "" {
		return nil, errors.New("invalid (empty) parameter name")
	}

	// find parameter by name
	k, ok := modelDef.ParamByName(name)
	if !ok {
		return nil, errors.New("parameter not found: " + name)
	}
	param := &modelDef.Param[k]

	// for float model types use format if specified
	isUseFmt := param.typeOf.IsFloat() && doubleFmt != ""

	cvt := func(src interface{}, row []string) error {

		cell, ok := src.(CellParam)
		if !ok {
			return errors.New("invalid type, expected: parameter cell (internal error)")
		}

		n := len(cell.DimIds)
		if len(row) != n+2 {
			return errors.New("invalid size of csv row buffer, expected: " + strconv.Itoa(n+2))
		}

		row[0] = fmt.Sprint(cell.SubId)

		for k, e := range cell.DimIds {
			row[k+1] = fmt.Sprint(e)
		}
		if isUseFmt {
			row[n+1] = fmt.Sprintf(doubleFmt, cell.Value)
		} else {
			row[n+1] = fmt.Sprint(cell.Value)
		}
		return nil
	}

	return cvt, nil
}

// CsvToRow return converter from parameter cell (sub id, dimensions, value) to csv row []string.
//
// Converter will retrun error if len(row) not equal to number of fields in csv record.
// Double format string is used if parameter type is float, double, long double
// If dimension type is enum based then csv row is enum code and cell.DimIds is enum id.
// If parameter type is enum based then cell value is enum id and csv row value is enum code.
func (CellParam) CsvToRow(
	modelDef *ModelMeta, name string, doubleFmt string, valueName string) (
	func(interface{}, []string) error, error) {

	// validate parameters
	if modelDef == nil {
		return nil, errors.New("invalid (empty) model metadata, look like model not found")
	}
	if name == "" {
		return nil, errors.New("invalid (empty) parameter name")
	}

	// find parameter by name
	k, ok := modelDef.ParamByName(name)
	if !ok {
		return nil, errors.New("parameter not found: " + name)
	}
	param := &modelDef.Param[k]

	// for each dimension create converter from item id to code
	fd := make([]func(itemId int) (string, error), param.Rank)

	for k := 0; k < param.Rank; k++ {
		f, err := cvtItemIdToCode(name+"."+param.Dim[k].Name, param.Dim[k].typeOf, param.Dim[k].typeOf.Enum, false, 0)
		if err != nil {
			return nil, err
		}
		fd[k] = f
	}

	// if parameter value type is float then use format, if not empty
	isUseFmt := param.typeOf.IsFloat() && doubleFmt != ""

	// if parameter value type is enum-based then convert from enum id to code
	isUseEnum := !param.typeOf.IsBuiltIn()
	var fv func(itemId int) (string, error)

	if isUseEnum {
		f, err := cvtItemIdToCode(name, param.typeOf, param.typeOf.Enum, false, 0)
		if err != nil {
			return nil, err
		}
		fv = f
	}

	cvt := func(src interface{}, row []string) error {

		cell, ok := src.(CellParam)
		if !ok {
			return errors.New("invalid type, expected: parameter cell (internal error)")
		}

		n := len(cell.DimIds)
		if len(row) != n+2 {
			return errors.New("invalid size of csv row buffer, expected: " + strconv.Itoa(n+2))
		}

		row[0] = fmt.Sprint(cell.SubId)

		// convert dimension item id to code
		for k, e := range cell.DimIds {
			v, err := fd[k](e)
			if err != nil {
				return err
			}
			row[k+1] = v
		}

		// convert cell value:
		// if float then use format, if enum then find code by id, default: Sprint(value)
		if isUseFmt {
			row[n+1] = fmt.Sprintf(doubleFmt, cell.Value)
		}
		if !isUseFmt && isUseEnum {

			// depending on sql + driver it can be different type
			var iv int
			switch e := cell.Value.(type) {
			case int64:
				iv = int(e)
			case uint64:
				iv = int(e)
			case int32:
				iv = int(e)
			case uint32:
				iv = int(e)
			case int16:
				iv = int(e)
			case uint16:
				iv = int(e)
			case int8:
				iv = int(e)
			case uint8:
				iv = int(e)
			case uint:
				iv = int(e)
			case float32: // oracle (very unlikely)
				iv = int(e)
			case float64: // oracle (often)
				iv = int(e)
			case int:
				iv = e
			default:
				return errors.New("invalid parameter value type, expected: integer enum")
			}

			v, err := fv(int(iv))
			if err != nil {
				return err
			}
			row[n+1] = v
		} else {
			row[n+1] = fmt.Sprint(cell.Value)
		}

		return nil
	}

	return cvt, nil
}

// CsvToCell return closure to convert csv row []string to parameter cell (sub id, dimensions, value).
//
// It does retrun error if len(row) not equal to number of fields in cell db-record.
// If dimension type is enum based then csv row is enum code and cell.DimIds is enum id.
// If parameter type is enum based then cell value is enum id and csv row value is enum code.
func (CellParam) CsvToCell(
	modelDef *ModelMeta, name string, subCount int, valueName string) (
	func(row []string) (interface{}, error), error) {

	// validate parameters
	if modelDef == nil {
		return nil, errors.New("invalid (empty) model metadata, look like model not found")
	}
	if name == "" {
		return nil, errors.New("invalid (empty) parameter name")
	}

	// find parameter by name
	idx, ok := modelDef.ParamByName(name)
	if !ok {
		return nil, errors.New("parameter not found: " + name)
	}
	param := &modelDef.Param[idx]

	// for each dimension create converter from item code to id
	fd := make([]func(src string) (int, error), param.Rank)

	for k := 0; k < param.Rank; k++ {
		f, err := cvtItemCodeToId(name+"."+param.Dim[k].Name, param.Dim[k].typeOf, param.Dim[k].typeOf.Enum, false, 0)
		if err != nil {
			return nil, err
		}
		fd[k] = f
	}

	// cell value converter: float, bool, string or integer by default
	var fc func(src string) (interface{}, error)
	var fe func(src string) (int, error)
	isEnum := !param.typeOf.IsBuiltIn()

	switch {
	case isEnum:
		f, err := cvtItemCodeToId(name, param.typeOf, param.typeOf.Enum, false, 0)
		if err != nil {
			return nil, err
		}
		fe = f
	case param.typeOf.IsFloat():
		fc = func(src string) (interface{}, error) { return strconv.ParseFloat(src, 64) }
	case param.typeOf.IsBool():
		fc = func(src string) (interface{}, error) { return strconv.ParseBool(src) }
	case param.typeOf.IsString():
		fc = func(src string) (interface{}, error) { return src, nil }
	case param.typeOf.IsInt():
		fc = func(src string) (interface{}, error) { return strconv.Atoi(src) }
	default:
		return nil, errors.New("invalid (not supported) parameter type: " + name)
	}

	// do conversion
	cvt := func(row []string) (interface{}, error) {

		// make conversion buffer and check input csv row size
		cell := CellParam{cellValue: cellValue{cellDims: cellDims{DimIds: make([]int, param.Rank)}}}

		n := len(cell.DimIds)
		if len(row) != n+2 {
			return nil, errors.New("invalid size of csv row, expected: " + strconv.Itoa(n+2))
		}

		// subvalue number
		nSub, err := strconv.Atoi(row[0])
		if err != nil {
			return nil, err
		}
		if nSub < 0 || nSub >= subCount {
			return nil, errors.New("invalid sub-value id: " + strconv.Itoa(nSub) + " parameter: " + name)
		}
		cell.SubId = nSub

		// convert dimensions: enum code to enum id or integer value for simple type dimension
		for k := range cell.DimIds {
			i, err := fd[k](row[k+1])
			if err != nil {
				return nil, err
			}
			cell.DimIds[k] = i
		}

		// value conversion
		var v interface{}
		if isEnum {
			v, err = fe(row[n+1])
		} else {
			v, err = fc(row[n+1])
		}
		if err != nil {
			return nil, err
		}
		cell.Value = v

		return cell, nil
	}

	return cvt, nil
}