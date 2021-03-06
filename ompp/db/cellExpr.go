// Copyright (c) 2016 OpenM++
// This code is licensed under the MIT license (see LICENSE.txt for details)

package db

import (
	"errors"
	"fmt"
	"strconv"
)

// CellExpr is value of output table expression.
type CellExpr struct {
	cellIdValue     // dimensions and value
	ExprId      int // output table expression id
}

// CellCodeExpr is value of output table expression.
// Dimension(s) items are enum codes, not enum ids.
type CellCodeExpr struct {
	cellCodeValue     // dimensions as enum codes and value
	ExprId        int // output table expression id
}

// CsvFileName return file name of csv file to store output table expression rows
func (CellExpr) CsvFileName(modelDef *ModelMeta, name string, isIdCsv bool) (string, error) {

	// validate parameters
	if modelDef == nil {
		return "", errors.New("invalid (empty) model metadata, look like model not found")
	}
	if name == "" {
		return "", errors.New("invalid (empty) output table name")
	}

	// find output table by name
	k, ok := modelDef.OutTableByName(name)
	if !ok {
		return "", errors.New("output table not found: " + name)
	}

	if isIdCsv {
		return modelDef.Table[k].Name + ".id.csv", nil
	}
	return modelDef.Table[k].Name + ".csv", nil
}

// CsvHeader retrun first line for csv file: column names.
// It is like: expr_name,dim0,dim1,expr_value
// or if isIdHeader is true: expr_id,dim0,dim1,expr_value
func (CellExpr) CsvHeader(modelDef *ModelMeta, name string, isIdHeader bool, valueName string) ([]string, error) {

	// validate parameters
	if modelDef == nil {
		return nil, errors.New("invalid (empty) model metadata, look like model not found")
	}
	if name == "" {
		return nil, errors.New("invalid (empty) output table name")
	}

	// find output table by name
	k, ok := modelDef.OutTableByName(name)
	if !ok {
		return nil, errors.New("output table not found: " + name)
	}
	table := &modelDef.Table[k]

	// make first line columns
	h := make([]string, table.Rank+2)

	if isIdHeader {
		h[0] = "expr_id"
	} else {
		h[0] = "expr_name"
	}
	for k := range table.Dim {
		h[k+1] = table.Dim[k].Name
	}
	h[table.Rank+1] = "expr_value"

	return h, nil
}

// CsvToIdRow return converter from output table cell (expr_id, dimensions, value) to csv row []string.
//
// Converter simply does Sprint() for each dimension item id, expression id and value.
// Converter will retrun error if len(row) not equal to number of fields in csv record.
// Double format string is used if parameter type is float, double, long double
func (CellExpr) CsvToIdRow(
	modelDef *ModelMeta, name string, doubleFmt string, valueName string,
) (
	func(interface{}, []string) error, error) {

	cvt := func(src interface{}, row []string) error {

		cell, ok := src.(CellExpr)
		if !ok {
			return errors.New("invalid type, expected: output table expression cell (internal error)")
		}

		n := len(cell.DimIds)
		if len(row) != n+2 {
			return errors.New("invalid size of csv row buffer, expected: " + strconv.Itoa(n+2))
		}

		row[0] = fmt.Sprint(cell.ExprId)

		for k, e := range cell.DimIds {
			row[k+1] = fmt.Sprint(e)
		}

		// use "null" string for db NULL values and format for model float types
		if cell.IsNull {
			row[n+1] = "null"
		} else {
			if doubleFmt != "" {
				row[n+1] = fmt.Sprintf(doubleFmt, cell.Value)
			} else {
				row[n+1] = fmt.Sprint(cell.Value)
			}
		}
		return nil
	}

	return cvt, nil
}

// CsvToRow return converter from output table cell (expr_id, dimensions, value)
// to csv row []string (expr_name, dimensions, value).
//
// Converter will retrun error if len(row) not equal to number of fields in csv record.
// Double format string is used if parameter type is float, double, long double.
// If dimension type is enum based then csv row is enum code and cell.DimIds is enum id.
func (CellExpr) CsvToRow(
	modelDef *ModelMeta, name string, doubleFmt string, valueName string,
) (
	func(interface{}, []string) error, error) {

	// validate parameters
	if modelDef == nil {
		return nil, errors.New("invalid (empty) model metadata, look like model not found")
	}
	if name == "" {
		return nil, errors.New("invalid (empty) output table name")
	}

	// find output table by name
	k, ok := modelDef.OutTableByName(name)
	if !ok {
		return nil, errors.New("output table not found: " + name)
	}
	table := &modelDef.Table[k]

	// for each dimension create converter from item id to code
	fd := make([]func(itemId int) (string, error), table.Rank)

	for k := 0; k < table.Rank; k++ {
		f, err := cvtItemIdToCode(name+"."+table.Dim[k].Name, table.Dim[k].typeOf, table.Dim[k].IsTotal)
		if err != nil {
			return nil, err
		}
		fd[k] = f
	}

	cvt := func(src interface{}, row []string) error {

		cell, ok := src.(CellExpr)
		if !ok {
			return errors.New("invalid type, expected: output table expression cell (internal error)")
		}

		n := len(cell.DimIds)
		if len(row) != n+2 {
			return errors.New("invalid size of csv row buffer, expected: " + strconv.Itoa(n+2))
		}

		row[0] = table.Expr[cell.ExprId].Name

		// convert dimension item id to code
		for k, e := range cell.DimIds {
			v, err := fd[k](e)
			if err != nil {
				return err
			}
			row[k+1] = v
		}

		// use "null" string for db NULL values and format for model float types
		if cell.IsNull {
			row[n+1] = "null"
		} else {
			if doubleFmt != "" {
				row[n+1] = fmt.Sprintf(doubleFmt, cell.Value)
			} else {
				row[n+1] = fmt.Sprint(cell.Value)
			}
		}
		return nil
	}

	return cvt, nil
}

// CsvToCell return closure to convert csv row []string to output table expression cell (dimensions and value).
//
// It does retrun error if len(row) not equal to number of fields in cell db-record.
// If dimension type is enum based then csv row is enum code and cell.DimIds is enum id.
func (CellExpr) CsvToCell(
	modelDef *ModelMeta, name string, subCount int, valueName string,
) (
	func(row []string) (interface{}, error), error) {

	// validate parameters
	if modelDef == nil {
		return nil, errors.New("invalid (empty) model metadata, look like model not found")
	}
	if name == "" {
		return nil, errors.New("invalid (empty) output table name")
	}

	// find output table by name
	k, ok := modelDef.OutTableByName(name)
	if !ok {
		return nil, errors.New("output table not found: " + name)
	}
	table := &modelDef.Table[k]

	// for each dimension create converter from item code to id
	fd := make([]func(src string) (int, error), table.Rank)

	for k := 0; k < table.Rank; k++ {
		f, err := cvtItemCodeToId(name+"."+table.Dim[k].Name, table.Dim[k].typeOf, table.Dim[k].IsTotal)
		if err != nil {
			return nil, err
		}
		fd[k] = f
	}

	// do conversion
	cvt := func(row []string) (interface{}, error) {

		// make conversion buffer and check input csv row size
		cell := CellExpr{cellIdValue: cellIdValue{DimIds: make([]int, table.Rank)}}

		n := len(cell.DimIds)
		if len(row) != n+2 {
			return nil, errors.New("invalid size of csv row, expected: " + strconv.Itoa(n+2))
		}

		// expression id by name
		cell.ExprId = -1
		for k := range table.Expr {
			if row[0] == table.Expr[k].Name {
				cell.ExprId = k
				break
			}
		}
		if cell.ExprId < 0 {
			return nil, errors.New("invalid expression name: " + row[0] + " output table: " + name)
		}

		// convert dimensions: enum code to enum id or integer value for simple type dimension
		for k := range cell.DimIds {
			i, err := fd[k](row[k+1])
			if err != nil {
				return nil, err
			}
			cell.DimIds[k] = i
		}

		// value conversion
		cell.IsNull = row[n+1] == "" || row[n+1] == "null"

		if cell.IsNull {
			cell.Value = 0.0
		} else {
			v, err := strconv.ParseFloat(row[n+1], 64)
			if err != nil {
				return nil, err
			}
			cell.Value = v
		}
		return cell, nil
	}

	return cvt, nil
}

// IdToCodeCell return converter from output table cell of ids: (expr_id, dimensions enum ids, value)
// to cell of codes: (expr_id, dimensions as enum codes, value).
//
// If dimension type is enum based then dimensions enum ids can be converted to enum code.
// If dimension type is simple (bool or int) then dimension value converted to string.
func (CellExpr) IdToCodeCell(
	modelDef *ModelMeta, name string,
) (
	func(interface{}) (interface{}, error), error) {

	// validate parameters
	if modelDef == nil {
		return nil, errors.New("invalid (empty) model metadata, look like model not found")
	}
	if name == "" {
		return nil, errors.New("invalid (empty) output table name")
	}

	// find output table by name
	k, ok := modelDef.OutTableByName(name)
	if !ok {
		return nil, errors.New("output table not found: " + name)
	}
	table := &modelDef.Table[k]

	// for each dimension create converter from item id to code
	fd := make([]func(itemId int) (string, error), table.Rank)

	for k := 0; k < table.Rank; k++ {
		f, err := cvtItemIdToCode(name+"."+table.Dim[k].Name, table.Dim[k].typeOf, table.Dim[k].IsTotal)
		if err != nil {
			return nil, err
		}
		fd[k] = f
	}

	// create cell converter
	cvt := func(src interface{}) (interface{}, error) {

		srcCell, ok := src.(CellExpr)
		if !ok {
			return nil, errors.New("invalid type, expected: output table expression cell (internal error)")
		}
		if len(srcCell.DimIds) != table.Rank {
			return nil, errors.New("invalid cell rank: " + strconv.Itoa(len(srcCell.DimIds)) + ", expected: " + strconv.Itoa(table.Rank))
		}

		dstCell := CellCodeExpr{
			cellCodeValue: cellCodeValue{
				Dims:   make([]string, table.Rank),
				IsNull: srcCell.IsNull,
				Value:  srcCell.Value,
			},
			ExprId: srcCell.ExprId,
		}

		// convert dimension item id to code
		for k := range srcCell.DimIds {
			v, err := fd[k](srcCell.DimIds[k])
			if err != nil {
				return nil, err
			}
			dstCell.Dims[k] = v
		}

		return dstCell, nil // converted OK
	}

	return cvt, nil
}
