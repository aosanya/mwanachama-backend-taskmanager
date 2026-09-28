package mwanachamataskmanager

import _ "embed"

//go:embed taskmanager.operations.json
var operationsJSON []byte

func Operations() []byte { return operationsJSON }
