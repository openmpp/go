// Copyright OpenM++
// This code is licensed under the MIT license (see LICENSE.txt for details)

package main

import (
	"database/sql"
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"strconv"

	_ "github.com/mattn/go-sqlite3"

	"github.com/openmpp/go/ompp/config"
	"github.com/openmpp/go/ompp/db"
	"github.com/openmpp/go/ompp/helper"
	"github.com/openmpp/go/ompp/omppLog"
)

// find model run row by digest, stamp or name, if rdsn is not "" empty, or by run id, if id > 0, or by first or last bool flag
func findRun(srcDb *sql.DB, modelId int, rdsn string, runId int, isFirst, isLast bool) (string, *db.RunRow, error) {

	if rdsn == "" && runId <= 0 && !isFirst && !isLast {
		return "", nil, nil
	}
	if rdsn != "" {
		r, e := db.GetRunByDigestStampName(srcDb, modelId, rdsn)
		return rdsn, r, e
	}
	if runId > 0 {
		r, e := db.GetRun(srcDb, runId)

		if e == nil && r != nil && r.ModelId != modelId {
			return "", nil, errors.New("Error: model run not found by id: " + strconv.Itoa(runId))
		}
		return strconv.Itoa(runId), r, e
	}
	if isFirst {
		r, e := db.GetFirstRun(srcDb, modelId)
		return "first model run", r, e
	}
	// else: must be last model run
	r, e := db.GetLastRun(srcDb, modelId)
	return "last model run", r, e
}

// compare model runs microdata and run results into csv or json files
func microdataCompare(srcDb *sql.DB, modelId int, runOpts *config.RunOptions) error {

	// find base model run
	msg, baseRun, err := findRun(srcDb, modelId, runOpts.String(runArgKey), runOpts.Int(runIdArgKey, 0), runOpts.Bool(runFirstArgKey), runOpts.Bool(runLastArgKey))
	if err != nil {
		return errors.New("Error at get base model run: " + msg + " " + err.Error())
	}
	if baseRun == nil {
		return errors.New("Error: base model run not found: " + msg)
	}
	if baseRun.Status != db.DoneRunStatus {
		return errors.New("Error: base model run not completed successfully: " + msg)
	}

	// make list of variant model runs
	varRunLst := []*db.RunRow{}

	// check variant run search results and push to vrarints list
	pushToVar := func(src string, m string, r *db.RunRow) error {

		if src != "" && r == nil {
			return errors.New("Error: variant model run not found: " + src)
		}
		if r.Status != db.DoneRunStatus {
			return errors.New("Error: variant model run not completed successfully: " + m)
		}
		if r.RunDigest == baseRun.RunDigest {
			omppLog.Log("Warning: skip variant run, it is the same as base run: ", src)
			return nil
		}

		// check if variant not already exist in the list of variants
		isFound := false
		for j := 0; !isFound && j < len(varRunLst); j++ {
			isFound = varRunLst[j].RunDigest == r.RunDigest
		}
		if !isFound {
			varRunLst = append(varRunLst, r)
		}
		return nil
	}

	// get variant runs from comma separarted list of digest, stamp or name
	if rdsnLst := helper.ParseCsvLine(runOpts.String(toRunsArgKey), ','); len(rdsnLst) > 0 {

		for _, rdsn := range rdsnLst {

			m, r, e := findRun(srcDb, modelId, rdsn, 0, false, false)
			if e != nil {
				return errors.New("Error at get variant model run: " + m + " " + e.Error())
			}
			if e = pushToVar(rdsn, m, r); e != nil {
				return e
			}
		}
	}
	// get variant runs from comma separarted list of run id's
	if idLst := helper.ParseCsvLine(runOpts.String(toRunIdsArgKey), ','); len(idLst) > 0 {

		for _, sId := range idLst {

			if sId == "" {
				continue
			}
			rId, e := strconv.Atoi(sId)
			if e != nil || rId <= 0 {
				return errors.New("Invalid variant model run id: " + sId)
			}

			m, r, e := findRun(srcDb, modelId, "", rId, false, false)
			if e != nil {
				return errors.New("Error at get variant model run: " + m + " " + e.Error())
			}
			if e = pushToVar(sId, m, r); e != nil {
				return e
			}
		}
	}
	// check if first run must be used as variant run
	if runOpts.Bool(toRunFirstArgKey) {

		m, r, e := findRun(srcDb, modelId, "", 0, true, false)
		if e != nil {
			return errors.New("Error at get first model run: " + m + " " + e.Error())
		}
		if e = pushToVar(m, m, r); e != nil {
			return e
		}
	}
	// check if last run must be used as variant run
	if runOpts.Bool(toRunLastArgKey) {

		m, r, e := findRun(srcDb, modelId, "", 0, false, true)
		if e != nil {
			return errors.New("Error at get last model run: " + m + " " + e.Error())
		}
		if e = pushToVar(m, m, r); e != nil {
			return e
		}
	}

	// get microdata entity, group by attributes and comparison calcultion(s)
	entityName := runOpts.String(entityArgKey)
	if entityName == "" {
		return errors.New("Invalid (empty) microdata entity name")
	}
	groupBy := helper.ParseCsvLine(runOpts.String(groupByArgKey), ',')
	if len(groupBy) <= 0 {
		return errors.New("Invalid (empty) microdata group by attributes")
	}
	cLst := helper.ParseCsvLine(runOpts.String(calcArgKey), ',')
	if len(cLst) <= 0 {
		return errors.New("Invalid (empty) microdata comparison calculation(s)")
	}

	// set aggregation expressions
	calcLt := db.CalculateMicroLayout{
		Calculation: []db.CalculateLayout{},
		GroupBy:     groupBy,
	}

	for j := range cLst {

		if cLst[j] != "" {
			calcLt.Calculation = append(calcLt.Calculation, db.CalculateLayout{
				Calculate: cLst[j],
				CalcId:    j + db.CALCULATED_ID_OFFSET,
				Name:      "ex_" + strconv.Itoa(j+db.CALCULATED_ID_OFFSET),
			})
		}
	}

	// get model metadata and find entity
	meta, err := db.GetModelById(srcDb, modelId)
	if err != nil {
		return errors.New("Error at get model metadata by id: " + strconv.Itoa(modelId) + ": " + err.Error())
	}

	// find model entity by entity name
	eIdx, ok := meta.EntityByName(entityName)
	if !ok {
		return errors.New("Error: model entity not found: " + entityName)
	}
	ent := &meta.Entity[eIdx]

	// create cell conveter to csv
	cvtMicro := db.CellMicroCalcConverter{
		CellEntityConverter: db.CellEntityConverter{
			ModelDef:  meta,
			Name:      entityName,
			IsIdCsv:   false, // use code, not id's
			DoubleFmt: theCfg.doubleFmt,
		},
		CalcMaps: db.EmptyCalcMaps(),
		GroupBy:  calcLt.GroupBy,
	}
	if e := cvtMicro.SetCalcIdNameMap(calcLt.Calculation); e != nil {
		return errors.New("Failed to create microdata aggregation converter to csv: " + entityName + ": " + e.Error())
	}

	// set run digests and run id's maps in the convereter
	cvtMicro.CalcMaps.IdToDigest[baseRun.RunId] = baseRun.RunDigest // add base run digest to converter
	cvtMicro.CalcMaps.DigestToId[baseRun.RunDigest] = baseRun.RunId

	runIds := make([]int, len(varRunLst))
	for k := 0; k < len(varRunLst); k++ {
		cvtMicro.CalcMaps.IdToDigest[varRunLst[k].RunId] = varRunLst[k].RunDigest // add run digest to converter
		cvtMicro.CalcMaps.DigestToId[varRunLst[k].RunDigest] = varRunLst[k].RunId
		runIds[k] = varRunLst[k].RunId
	}

	// find entity generation by entity name and validate entity generation: it must exist for base run and all variant runs
	//
	// get list of entity generations for base model run
	egLst, err := db.GetEntityGenList(srcDb, baseRun.RunId)
	if err != nil {
		return errors.New("Error at get run entities: " + entityName + ": " + strconv.Itoa(baseRun.RunId) + ": " + err.Error())
	}

	// find entity generation by entity id, as it is today model run has only one entity generation for each entity
	gIdx := -1
	for k := range egLst {

		if egLst[k].EntityId == ent.EntityId {
			gIdx = k
			break
		}
	}
	if gIdx < 0 {
		return errors.New("Error: model run entity generation not found: " + entityName + ": " + strconv.Itoa(baseRun.RunId))
	}
	entGen := &egLst[gIdx] // entity generation exists in the base run

	// collect generation attribues
	attrs := make([]db.EntityAttrRow, len(entGen.GenAttr))

	for k, ga := range entGen.GenAttr {

		aIdx, ok := ent.AttrByKey(ga.AttrId)
		if !ok {
			return errors.New("entity attribute not found by id: " + strconv.Itoa(ga.AttrId) + " " + entityName)
		}
		attrs[k] = ent.Attr[aIdx]
	}

	// find all run_entity rows for that entity generation
	runEnt, err := db.GetRunEntityGenByModel(srcDb, modelId)
	if err != nil {
		return errors.New("Error at get run entities by model id: " + strconv.Itoa(modelId) + ": " + err.Error())
	}

	n := 0
	for k := 0; k < len(runEnt); k++ {
		if runEnt[k].GenHid == entGen.GenHid {
			runEnt[n] = runEnt[k]
			n++
		}
	}
	runEnt = runEnt[:n]

	// validate entity generation: it in the base run and must exist for all variant runs
	cvtMicro.EntityGen = entGen

	for k := 0; k < len(runIds); k++ {
		isFound := false
		for j := 0; !isFound && j < len(runEnt); j++ {
			isFound = runEnt[j].RunId == runIds[k]
		}
		if !isFound {
			return errors.New("Failed to create microdata aggregation converter to csv, entity not found in the run: " + strconv.Itoa(runIds[k]) + ": " + entityName)
		}
	}

	// validate group by attributes
	for k := 0; k < len(calcLt.GroupBy); k++ {

		isFound := false
		for j := 0; !isFound && j < len(attrs); j++ {
			isFound = attrs[j].Name == calcLt.GroupBy[k]
		}
		if !isFound {
			return errors.New("Invalid group by attribute: " + entityName + "." + calcLt.GroupBy[k])
		}
	}

	// read microdata values, page size =0: read all values
	microLt := db.ReadMicroLayout{
		ReadLayout: db.ReadLayout{
			Name:           entityName,
			FromId:         baseRun.RunId,
			ReadPageLayout: db.ReadPageLayout{Offset: 0, Size: 0},
		},
		GenDigest: entGen.GenDigest,
	}

	// make csv header
	hdr, err := cvtMicro.CsvHeader()
	if err != nil {
		return errors.New("Failed to make microdata csv header: " + entityName + ": " + err.Error())
	}

	// create converter from db cell into csv row []string
	var cvtRow func(interface{}, []string) (bool, error)

	cvtRow, err = cvtMicro.ToCsvRow()
	if err != nil {
		return errors.New("Failed to create microdata converter to csv: " + entityName + ": " + err.Error())
	}

	// use specified file name or make default
	fp := ""
	if !theCfg.isNoFile {
		fp = theCfg.fileName
		if fp == "" {
			if theCfg.isJson {
				fp = entityName + ".json"
			} else {
				fp = entityName + ".csv"
			}
		}
		fp = filepath.Join(theCfg.dir, fp)
	}

	if theCfg.isNoFile {
		omppLog.Log("Do model-list")
	} else {
		omppLog.Log("Do model-list: " + fp)
	}
	// create csv file
	isFile := fp != ""
	var f *os.File

	if isFile {
		f, err = os.OpenFile(fp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
	}
	defer func() {
		if isFile {
			f.Close()
		}
	}()

	if isFile && theCfg.isWriteUtf8Bom { // if required then write utf-8 bom
		if _, err = f.Write(helper.Utf8bom); err != nil {
			return err
		}
	}

	// write csv header
	csvWr := csv.NewWriter(f)

	if err := csvWr.Write(hdr); err != nil {
		return errors.New("Error at csv write: " + entityName + ": " + err.Error())
	}

	// convert output table cell into []string and write line into csv file
	cs := make([]string, len(hdr))

	cvtWr := func(c interface{}) (bool, error) {

		// if converter return empty line then skip it
		isNotEmpty := true
		var e2 error = nil

		if isNotEmpty, e2 = cvtRow(c, cs); e2 != nil {
			return false, e2
		}
		if isNotEmpty {
			if e2 = csvWr.Write(cs); e2 != nil {
				return false, e2
			}
		}
		return true, nil
	}

	// read microdata values page
	_, err = db.ReadMicrodataCalculateTo(srcDb, meta, &microLt, &calcLt, runIds, cvtWr)
	if err != nil {
		return errors.New("Error at microdata run comparison: " + entityName + ": " + microLt.GenDigest + ": " + err.Error())
	}

	csvWr.Flush() // flush csv to response

	return nil
}
