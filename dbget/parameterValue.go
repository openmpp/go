// Copyright OpenM++
// This code is licensed under the MIT license (see LICENSE.txt for details)

package main

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strconv"

	"github.com/openmpp/go/ompp/config"
	"github.com/openmpp/go/ompp/db"
	"github.com/openmpp/go/ompp/omppLog"
)

// get model run parameter values and write run results into csv or tsv file.
func parameterRunValue(srcDb *sql.DB, modelId int, runOpts *config.RunOptions) error {

	// get model metadata
	meta, err := db.GetModelById(srcDb, modelId)
	if err != nil {
		return errors.New("Error at get model metadata by id: " + strconv.Itoa(modelId) + ": " + err.Error())
	}

	// find model run
	msg, run, err := findRun(srcDb, modelId, runOpts.String(runArgKey), runOpts.Int(runIdArgKey, 0), runOpts.Bool(runFirstArgKey), runOpts.Bool(runLastArgKey))
	if err != nil {
		return errors.New("Error at get model run: " + msg + " " + err.Error())
	}
	if run == nil {
		return errors.New("Error: model run not found")
	}
	if run.Status != db.DoneRunStatus {
		return errors.New("Error: model run not completed successfully: " + run.Name)
	}

	// write parameter values to csv or tsv file
	name := runOpts.String(paramArgKey)
	fp := ""

	if theCfg.isConsole {
		omppLog.Log("Do ", theCfg.action, " ", name)
	} else {

		fp = theCfg.fileName
		if fp == "" {
			fp = name + extByKind()
		}
		fp = filepath.Join(theCfg.dir, fp)

		omppLog.Log("Do ", theCfg.action, ": "+fp)
	}

	return parameterValue(srcDb, meta, name, run.RunId, false, fp, false, nil)
}

// get workset parameter values and write run results into csv or tsv file.
func parameterWsValue(srcDb *sql.DB, modelId int, runOpts *config.RunOptions) error {

	// get model metadata and find parameter
	meta, err := db.GetModelById(srcDb, modelId)
	if err != nil {
		return errors.New("Error at get model metadata by id: " + strconv.Itoa(modelId) + ": " + err.Error())
	}

	paramName := runOpts.String(paramArgKey)
	idx, ok := meta.ParamByName(paramName)
	if !ok {
		return errors.New("model parameter not found: " + paramName)
	}

	// find workset, it must be readonly and check if parameter exists in that workset
	wsRow, err := findWs(srcDb, modelId, runOpts)
	if err != nil {
		return err
	}

	nSub, _, err := db.GetWorksetParam(srcDb, wsRow.SetId, meta.Param[idx].ParamHid)
	if err != nil {
		return errors.New("Error at getting workset parameters list: " + wsRow.Name + ": " + err.Error())
	}
	if nSub <= 0 {
		return errors.New("Workset: " + wsRow.Name + " must contain parameter: " + paramName)
	}

	// write parameter values to csv or tsv file
	fp := ""
	if theCfg.isConsole {
		omppLog.Log("Do ", theCfg.action, " ", paramName)
	} else {

		fp = theCfg.fileName
		if fp == "" {
			fp = paramName + extByKind()
		}
		fp = filepath.Join(theCfg.dir, fp)

		omppLog.Log("Do ", theCfg.action, ": "+fp)
	}

	return parameterValue(srcDb, meta, paramName, wsRow.SetId, true, fp, false, nil)
}

// read model run parameter values and write run results into csv or tsv file.
// It can be compatibility view parameter csv file with header Dim0,Dim1,....,Value
// or normal csv file: sub_id,dim0,dim1,param_value.
// For compatibilty view parameter csv shold skip sub_id column
func parameterValue(srcDb *sql.DB, meta *db.ModelMeta, name string, fromId int, isFromSet bool, path string, isOld bool, csvHdr []string) error {

	if name == "" {
		return errors.New("Invalid (empty) parameter name")
	}
	if meta == nil {
		return errors.New("Invalid (empty) model metadata")
	}
	_, ok := meta.ParamByName(name)
	if !ok {
		return errors.New("Error: model parameter not found: " + name)
	}

	// make csv header
	// create converter from db cell into csv row []string
	var err error
	hdr := []string{}
	var cvtRow func(interface{}, []string) (bool, error)

	cvtParam := &db.CellParamConverter{
		ModelDef:  meta,
		Name:      name,
		IsIdCsv:   theCfg.isIdCsv,
		DoubleFmt: theCfg.doubleFmt,
	}
	paramLt := db.ReadParamLayout{
		IsFromSet: isFromSet,
		ReadLayout: db.ReadLayout{
			Name:   name,
			FromId: fromId,
		}}

	if theCfg.isNoLang || theCfg.isIdCsv {

		hdr, err = cvtParam.CsvHeader()
		if err != nil {
			return errors.New("Failed to make parameter csv header: " + name + ": " + err.Error())
		}
		if theCfg.isIdCsv {
			cvtRow, err = cvtParam.ToCsvIdRow()
		} else {
			cvtRow, err = cvtParam.ToCsvRow()
		}
		if err != nil {
			return errors.New("Failed to create parameter converter to csv: " + name + ": " + err.Error())
		}

	} else { // get language-specific metadata

		txt, err := db.GetModelText(srcDb, meta.Model.ModelId, theCfg.lang, true)
		if err != nil {
			return errors.New("Error at get model text metadata: " + err.Error())
		}

		cvtLoc := &db.CellParamLocaleConverter{
			CellParamConverter: *cvtParam,
			Lang:               theCfg.lang,
			EnumTxt:            txt.TypeEnumTxt,
		}

		hdr, err = cvtLoc.CsvHeader()
		if err != nil {
			return errors.New("Failed to make parameter csv header: " + name + ": " + err.Error())
		}
		cvtRow, err = cvtLoc.ToCsvRow()
		if err != nil {
			return errors.New("Failed to create parameter converter to csv: " + name + ": " + err.Error())
		}
	}

	// start csv output to file or console
	f, csvWr, err := createCsvWriter(path)
	if err != nil {
		return err
	}
	isFile := f != nil

	defer func() {
		if isFile {
			f.Close()
		}
	}()

	// write csv header, check if there is a custom header supplied
	h := hdr
	if len(csvHdr) > 0 {
		h = csvHdr
	}
	if err := csvWr.Write(h); err != nil {
		return errors.New("Error at csv write: " + name + ": " + err.Error())
	}

	// convert cell into []string and write line into csv file
	cs := make([]string, len(hdr))

	cvtWr := func(c interface{}) (bool, error) {

		// if converter return empty line then skip it
		isNotEmpty := false
		var e2 error = nil

		if isNotEmpty, e2 = cvtRow(c, cs); e2 != nil {
			return false, e2
		}
		if isNotEmpty {
			if !isOld {
				e2 = csvWr.Write(cs)
			} else {
				e2 = csvWr.Write(cs[1:]) // compatibility view: skip sub_id column
			}
		}
		return e2 == nil, e2
	}

	// read parameter values page
	_, err = db.ReadParameterTo(srcDb, meta, &paramLt, cvtWr)
	if err != nil {
		return errors.New("Error at parameter output: " + name + ": " + err.Error())
	}

	csvWr.Flush() // flush csv to response

	return nil
}
