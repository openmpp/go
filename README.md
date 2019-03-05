# [OpenM++](http://www.openmpp.org/) project: oms web-service, dbcopy utility and Go libraries

This repository is a part of [OpenM++](http://www.openmpp.org/) open source microsimulation platform.
It contains oms web-service, dbcopy utility and openM++ Go libraries.

## Build

```
go get github.com/openmpp/go/dbcopy
go install github.com/openmpp/go/dbcopy

go get github.com/openmpp/go/oms
go install github.com/openmpp/go/oms
```

By default only SQLite database supported. 
If you want to use other database vendors (Microsoft SQL, MySQL, PostgreSQL, IBM DB2, Oracle) then comile dbcopy with ODBC support:

```
go install -tags odbc github.com/openmpp/go/dbcopy
```

Please visit our [wiki](http://www.openmpp.org/wiki/) for more inforamition.
