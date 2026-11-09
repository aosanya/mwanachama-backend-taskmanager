package mwanachamataskmanager

import "github.com/aosanya/mwanachama-backend-taskmanager/models"

type (
	BacklogAccess = models.BacklogAccess
	GroupAccess   = models.GroupAccess
	AccessLevel   = models.AccessLevel
)

const (
	AccessSee  = models.AccessSee
	AccessMove = models.AccessMove
)
