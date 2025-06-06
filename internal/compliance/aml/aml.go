package aml

import "github.com/Aidin1998/finalex/internal/risk/compliance/aml"

type (
	AMLService              = aml.AMLService
	TransactionCheckRequest = aml.TransactionCheckRequest
	AMLResult               = aml.AMLResult
	UserAMLResult           = aml.UserAMLResult
	RiskProfile             = aml.RiskProfile
	TransactionLimits       = aml.TransactionLimits
	BasicAMLService         = aml.BasicAMLService
)

var NewBasicAMLService = aml.NewBasicAMLService
