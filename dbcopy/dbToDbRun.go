// Copyright (c) 2016 OpenM++
// This code is licensed under the MIT license (see LICENSE.txt for details)

package main

import (
	"database/sql"
	"errors"
	"strconv"

	"github.com/openmpp/go/ompp/config"
	"github.com/openmpp/go/ompp/db"
	"github.com/openmpp/go/ompp/omppLog"
)

// copy model run from source database to destination database
func dbToDbRun(modelName string, modelDigest string, runOpts *config.RunOptions) error {

	// validate source and destination
	inpConnStr := runOpts.String(dbConnStrArgKey)
	inpDriver := runOpts.String(dbDriverArgKey)
	outConnStr := runOpts.String(toDbConnStrArgKey)
	outDriver := runOpts.String(toDbDriverArgKey)

	if inpConnStr == outConnStr && inpDriver == outDriver {
		return errors.New("source same as destination: cannot overwrite model in database")
	}

	// open source database connection and check is it valid
	cs, dn := db.IfEmptyMakeDefault(modelName, inpConnStr, inpDriver)
	srcDb, _, err := db.Open(cs, dn, false)
	if err != nil {
		return err
	}
	defer srcDb.Close()

	if err := db.CheckOpenmppSchemaVersion(srcDb); err != nil {
		return err
	}

	// open destination database and check is it valid
	cs, dn = db.IfEmptyMakeDefault(modelName, outConnStr, outDriver)
	dstDb, _, err := db.Open(cs, dn, true)
	if err != nil {
		return err
	}
	defer dstDb.Close()

	if err := db.CheckOpenmppSchemaVersion(dstDb); err != nil {
		return err
	}

	// source: get model metadata
	srcModel, err := db.GetModel(srcDb, modelName, modelDigest)
	if err != nil {
		return err
	}
	modelName = srcModel.Model.Name // set model name: it can be empty and only model digest specified

	// find source model run metadata by id, run digest or name
	runId, runDigest, runName, isFirst, isLast := runIdDigestNameFromOptions(runOpts)
	if runId < 0 || runId == 0 && runName == "" && runDigest == "" && !isFirst && !isLast {
		return errors.New("dbcopy invalid argument(s) run id: " + runOpts.String(runIdArgKey) + ", run name: " + runOpts.String(runNameArgKey) + ", run digest: " + runOpts.String(runDigestArgKey))
	}
	runRow, e := findModelRunByIdDigestName(srcDb, srcModel.Model.ModelId, runId, runDigest, runName, isFirst, isLast)
	if e != nil {
		return e
	}
	if runRow == nil {
		return errors.New("model run not found: " + runOpts.String(runIdArgKey) + " " + runOpts.String(runNameArgKey) + " " + runOpts.String(runDigestArgKey))
	}

	// check is this run belong to the source model
	if runRow.ModelId != srcModel.Model.ModelId {
		return errors.New("model run " + strconv.Itoa(runRow.RunId) + " " + runRow.Name + " " + runRow.RunDigest + " does not belong to model " + modelName + " " + modelDigest)
	}

	// run must be completed: status success, error or exit
	if !db.IsRunCompleted(runRow.Status) {
		return errors.New("model run not completed: " + strconv.Itoa(runRow.RunId) + " " + runRow.Name)
	}

	// get full model run metadata
	meta, err := db.GetRunFullText(srcDb, runRow, "")
	if err != nil {
		return err
	}

	// destination: get model metadata
	dstModel, err := db.GetModel(dstDb, modelName, modelDigest)
	if err != nil {
		return err
	}

	// destination: get list of languages
	dstLang, err := db.GetLanguages(dstDb)
	if err != nil {
		return err
	}

	// convert model run db rows into "public" format
	// and copy source model run metadata, parameter values, output results into destination database
	pub, err := meta.ToPublic(srcDb, srcModel)
	if err != nil {
		return err
	}
	dblFmt := runOpts.String(doubleFormatArgKey)

	_, err = copyRunDbToDb(srcDb, dstDb, srcModel, dstModel, meta.Run.RunId, pub, dstLang, dblFmt)
	if err != nil {
		return err
	}

	return nil
}

// copyRunListDbToDb do copy all model runs parameters and output tables from source to destination database
// Double format is used for float model types digest calculation, if non-empty format supplied
func copyRunListDbToDb(
	srcDb *sql.DB, dstDb *sql.DB, srcModel *db.ModelMeta, dstModel *db.ModelMeta, dstLang *db.LangMeta, doubleFmt string) error {

	// source: get all successfully completed model runs in all languages
	srcRl, err := db.GetRunFullTextList(srcDb, srcModel.Model.ModelId, true, "")
	if err != nil {
		return err
	}
	if len(srcRl) <= 0 {
		return nil
	}

	// copy all run metadata, run parameters, output accumulators and expressions from source to destination
	// model run "public" format is used
	for k := range srcRl {

		// convert model db rows into "public"" format
		pub, err := srcRl[k].ToPublic(srcDb, srcModel)
		if err != nil {
			return err
		}

		// save into destination database
		_, err = copyRunDbToDb(srcDb, dstDb, srcModel, dstModel, srcRl[k].Run.RunId, pub, dstLang, doubleFmt)
		if err != nil {
			return err
		}
	}
	return nil
}

// copyRunDbToDb do copy model run metadata, run parameters and output tables from source to destination database
// it return destination run id (run id in destination database)
func copyRunDbToDb(
	srcDb *sql.DB, dstDb *sql.DB, srcModel *db.ModelMeta, dstModel *db.ModelMeta, srcId int, pub *db.RunPub, dstLang *db.LangMeta, doubleFmt string) (int, error) {

	// validate parameters
	if pub == nil {
		return 0, errors.New("invalid (empty) source model run metadata, source run not found or not exists")
	}

	// destination: convert from "public" format into destination db rows
	dstRun, err := pub.FromPublic(dstDb, dstModel)
	if err != nil {
		return 0, err
	}

	// destination: save model run metadata
	isExist, err := dstRun.UpdateRun(dstDb, dstModel, dstLang, doubleFmt)
	if err != nil {
		return 0, err
	}
	dstId := dstRun.Run.RunId
	if isExist { // exit if model run already exist
		omppLog.Log("Model run ", srcId, " ", pub.Name, " already exists as ", dstId)
		return dstId, nil
	}

	// copy all run parameters, output accumulators and expressions from source to destination
	omppLog.Log("Model run from ", srcId, " ", pub.Name, " to ", dstId)

	paramLt := db.ReadParamLayout{ReadLayout: db.ReadLayout{FromId: srcId}}
	dstParamLt := db.WriteParamLayout{
		WriteLayout: db.WriteLayout{ToId: dstId},
		DoubleFmt:   doubleFmt,
		IsToRun:     true}

	// copy all parameters values for that model run
	for j := range srcModel.Param {

		// source: read parameter values
		paramLt.Name = srcModel.Param[j].Name

		cLst, _, err := db.ReadParameter(srcDb, srcModel, &paramLt)
		if err != nil {
			return 0, err
		}
		if cLst.Len() <= 0 { // parameter data must exist for all parameters
			return 0, errors.New("missing run parameter values " + paramLt.Name + " run id: " + strconv.Itoa(paramLt.FromId))
		}

		// destination: insert parameter values in model run
		dstParamLt.Name = dstModel.Param[j].Name
		dstParamLt.SubCount = dstRun.Param[j].SubCount

		if err = db.WriteParameter(dstDb, dstModel, &dstParamLt, cLst); err != nil {
			return 0, err
		}
	}

	// copy all output tables values for that model run, if the table included in run results
	tblLt := db.ReadTableLayout{ReadLayout: db.ReadLayout{FromId: srcId}}
	dstTblLt := db.WriteTableLayout{
		WriteLayout: db.WriteLayout{ToId: dstId},
		DoubleFmt:   doubleFmt}

	for j := range srcModel.Table {

		// check if table exist in model run results
		var isFound bool
		for k := range pub.Table {
			isFound = pub.Table[k].Name == srcModel.Table[j].Name
			if isFound {
				break
			}
		}
		if !isFound {
			continue // skip table: it is suppressed and not in run results
		}

		// source: read output table accumulator
		tblLt.Name = srcModel.Table[j].Name
		tblLt.IsAccum = true

		acLst, _, err := db.ReadOutputTable(srcDb, srcModel, &tblLt)
		if err != nil {
			return 0, err
		}

		// source: read output table expression values
		tblLt.IsAccum = false

		ecLst, _, err := db.ReadOutputTable(srcDb, srcModel, &tblLt)
		if err != nil {
			return 0, err
		}

		// insert output table values (accumulators and expressions) in model run
		dstTblLt.Name = dstModel.Table[j].Name
		if err = db.WriteOutputTable(dstDb, dstModel, &dstTblLt, acLst, ecLst); err != nil {
			return 0, err
		}
	}

	return dstId, nil
}
