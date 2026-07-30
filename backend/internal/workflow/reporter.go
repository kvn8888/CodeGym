package workflow

import "context"

type reporterContextKey struct{}

type Reporter struct {
	service   *Service
	scope     identity
	operation Operation
}

func WithReporter(ctx context.Context, reporter *Reporter) context.Context {
	if reporter == nil {
		return ctx
	}
	return context.WithValue(ctx, reporterContextKey{}, reporter)
}

func ReporterFromContext(ctx context.Context) *Reporter {
	reporter, _ := ctx.Value(reporterContextKey{}).(*Reporter)
	return reporter
}

func (r *Reporter) Operation() Operation {
	if r == nil {
		return Operation{}
	}
	return r.operation
}

func (r *Reporter) Report(
	ctx context.Context,
	stepID string,
	status Status,
	metadata map[string]any,
	terminal bool,
) error {
	if r == nil || r.service == nil {
		return nil
	}
	operation, _, err := r.service.appendScoped(
		ctx, r.scope, r.operation.ID, stepID, status, metadata, terminal,
	)
	if err == nil {
		r.operation = operation
	}
	return err
}

func (r *Reporter) ReportDetached(
	stepID string,
	status Status,
	metadata map[string]any,
	terminal bool,
) error {
	return r.Report(context.Background(), stepID, status, metadata, terminal)
}
