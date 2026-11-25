package mwanachamataskmanager

import "context"

func UpdateRuleFromRead(mgr TaskManager, ctx context.Context, read RuleRecord, patch RulePatch) (RuleRecord, error) {
	return mgr.(*taskManager).updateRead(ctx, read, patch)
}

func SetRuleStatusFromRead(mgr TaskManager, ctx context.Context, read RuleRecord, status string) (RuleRecord, error) {
	return mgr.(*taskManager).setStatusRead(ctx, read, status)
}

func SupersedeRuleFromRead(mgr TaskManager, ctx context.Context, read, replacement RuleRecord) (RuleRecord, error) {
	return mgr.(*taskManager).supersedeRead(ctx, read, replacement)
}

func AnswerQuestionFromRead(mgr TaskManager, ctx context.Context, question, decision RuleRecord) (RuleRecord, error) {
	return mgr.(*taskManager).answerRead(ctx, question, decision)
}
