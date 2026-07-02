package tasks

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sheets"
)

// importChunkSize is how many rows are inserted per transaction; each flush
// bumps processed_rows and publishes one import_progress event.
const importChunkSize = 500

// importScan is the result of the counting/sampling pass over the CSV.
type importScan struct {
	header      []string
	sample      [][]string // first importInferenceSampleRows data rows
	totalRows   int64
	columnCount int
}

// run executes one attempt: wipe any partial rows (retry clean slate), scan
// the object to count rows and sample for inference, fail fast on the
// rows-per-page cap (plan §2d), resolve or create the target fields, then
// stream-insert in chunks and record the csv_import operation on completion.
func (h *SheetCSVImportHandler) run(ctx context.Context, job *model.SheetImportJob) error {
	if h.storage == nil {
		return fmt.Errorf("sheet csv import: storage reader is not configured")
	}
	opts, err := parseImportOptions(job.Options)
	if err != nil {
		return err
	}
	if err := h.svc.ResetImportRows(ctx, job); err != nil {
		return err
	}

	scan, err := h.scanCSV(ctx, job, opts)
	if err != nil {
		return err
	}
	existing, err := h.svc.ActiveRowCount(ctx, job.OrgID, job.PageID)
	if err != nil {
		return err
	}
	if int(existing)+int(scan.totalRows) > sheets.MaxRowsPerPage {
		return &sheets.LimitError{Limit: "rows per page", Max: sheets.MaxRowsPerPage, Actual: int(existing) + int(scan.totalRows)}
	}

	if err := h.svc.MarkImportJobRunning(ctx, job, scan.totalRows); err != nil {
		return err
	}
	h.svc.PublishImportProgress(ctx, job)

	targets, err := h.columnTargets(ctx, job, opts, scan)
	if err != nil {
		return err
	}
	if scan.totalRows > 0 {
		if err := h.processRows(ctx, job, opts, targets); err != nil {
			return err
		}
	}
	if err := h.svc.CompleteImportJob(ctx, job); err != nil {
		return err
	}
	h.svc.PublishImportProgress(ctx, job)
	return nil
}

// scanCSV streams the object once to count data rows, capture the header,
// and buffer the inference sample. Nothing is inserted here, so a file that
// would blow the row cap is rejected before any write.
func (h *SheetCSVImportHandler) scanCSV(ctx context.Context, job *model.SheetImportJob, opts *sheetImportOptions) (*importScan, error) {
	body, err := h.storage.Open(ctx, job.ObjectKey)
	if err != nil {
		return nil, fmt.Errorf("open csv object %q: %w", job.ObjectKey, err)
	}
	defer body.Close()

	reader := newImportCSVReader(body, opts.delimiter)
	scan := &importScan{}
	first := true
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, wrapCSVError(err)
		}
		if first && opts.hasHeader {
			scan.header = append([]string(nil), record...)
			scan.columnCount = len(record)
			first = false
			continue
		}
		first = false
		scan.totalRows++
		if len(record) > scan.columnCount {
			scan.columnCount = len(record)
		}
		if len(scan.sample) < importInferenceSampleRows {
			scan.sample = append(scan.sample, append([]string(nil), record...))
		}
	}
	return scan, nil
}

// columnTargets resolves one target field ID per CSV column ("" = skip).
// With a field_mapping the columns must land on existing fields; otherwise
// fields are created through the service (field_change semantics apply) with
// types inferred from the sample. Creation reuses an existing same-named
// field so a retried attempt never duplicates columns.
func (h *SheetCSVImportHandler) columnTargets(ctx context.Context, job *model.SheetImportJob, opts *sheetImportOptions, scan *importScan) ([]string, error) {
	if scan.columnCount == 0 {
		return nil, nil
	}
	fields, err := h.svc.ImportPageFields(ctx, job.OrgID, job.PageID)
	if err != nil {
		return nil, err
	}
	if len(opts.fieldMapping) > 0 {
		return resolveMappedColumns(opts, scan.header, scan.columnCount, fields)
	}

	byName := make(map[string]string, len(fields))
	for _, field := range fields {
		byName[field.Name] = field.ID
	}
	types := inferColumnTypes(scan.sample, scan.columnCount)
	actor := sheets.Actor{AgentID: job.CreatedByAgentID, UserID: job.CreatedByUserID}
	targets := make([]string, scan.columnCount)
	for i, fieldType := range types {
		name := importColumnName(scan.header, i, opts.hasHeader)
		if existingID, ok := byName[name]; ok {
			targets[i] = existingID
			continue
		}
		field, err := h.svc.CreateField(ctx, job.OrgID, job.PageID, sheets.FieldSpec{
			Name: name,
			Type: fieldType,
		}, actor)
		if err != nil {
			return nil, err
		}
		byName[name] = field.ID
		targets[i] = field.ID
	}
	return targets, nil
}

// processRows streams the object a second time and inserts data rows in
// chunks of importChunkSize, publishing progress after each committed chunk.
func (h *SheetCSVImportHandler) processRows(ctx context.Context, job *model.SheetImportJob, opts *sheetImportOptions, targets []string) error {
	body, err := h.storage.Open(ctx, job.ObjectKey)
	if err != nil {
		return fmt.Errorf("open csv object %q: %w", job.ObjectKey, err)
	}
	defer body.Close()

	reader := newImportCSVReader(body, opts.delimiter)
	reader.ReuseRecord = true
	chunk := make([]sheets.RowInsert, 0, importChunkSize)
	first := true
	var rowNum int64
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return wrapCSVError(err)
		}
		if first && opts.hasHeader {
			first = false
			continue
		}
		first = false
		rowNum++
		data := make(map[string]any, len(record))
		for i, raw := range record {
			if i >= len(targets) || targets[i] == "" {
				continue
			}
			value := strings.TrimSpace(raw)
			if value == "" {
				continue
			}
			data[targets[i]] = value
		}
		chunk = append(chunk, sheets.RowInsert{Data: data})
		if len(chunk) >= importChunkSize {
			if err := h.flushChunk(ctx, job, chunk, rowNum); err != nil {
				return err
			}
			chunk = chunk[:0]
		}
	}
	if len(chunk) > 0 {
		return h.flushChunk(ctx, job, chunk, rowNum)
	}
	return nil
}

func (h *SheetCSVImportHandler) flushChunk(ctx context.Context, job *model.SheetImportJob, chunk []sheets.RowInsert, throughRow int64) error {
	if _, err := h.svc.InsertImportRows(ctx, job, chunk); err != nil {
		return fmt.Errorf("import rows through data row %d: %w", throughRow, err)
	}
	h.svc.PublishImportProgress(ctx, job)
	return nil
}

// newImportCSVReader builds a lenient stdlib CSV reader: ragged rows are
// allowed (FieldsPerRecord unenforced) and LazyQuotes tolerates stray quotes
// common in real exports.
func newImportCSVReader(r io.Reader, delimiter rune) *csv.Reader {
	reader := csv.NewReader(r)
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	return reader
}

// wrapCSVError classifies parse failures as terminal csvFormatError; other
// read failures (network, storage) stay transient and retry.
func wrapCSVError(err error) error {
	var parseErr *csv.ParseError
	if errors.As(err, &parseErr) {
		return &csvFormatError{line: int64(parseErr.Line), err: err}
	}
	return fmt.Errorf("read csv: %w", err)
}
