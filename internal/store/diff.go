package store

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"

	"github.com/davemorin/supacrawl/internal/postgres"
)

type DiffResult struct {
	Current         ArchiveRef        `json:"current"`
	Baseline        ArchiveRef        `json:"baseline"`
	ProjectMismatch bool              `json:"project_mismatch"`
	Tables          TableDiff         `json:"tables"`
	Columns         ColumnDiff        `json:"columns"`
	Indexes         IndexDiff         `json:"indexes"`
	Constraints     ConstraintDiff    `json:"constraints"`
	Policies        PolicyDiff        `json:"policies"`
	Functions       FunctionDiff      `json:"functions"`
	Triggers        TriggerDiff       `json:"triggers"`
	Extensions      ExtensionDiff     `json:"extensions"`
	StorageBuckets  StorageBucketDiff `json:"storage_buckets"`
	TableRows       TableRowCountDiff `json:"table_rows"`
}

type ArchiveRef struct {
	Path        string    `json:"path"`
	ProjectID   string    `json:"project_id"`
	CollectedAt time.Time `json:"collected_at"`
}

type TableDiff struct {
	Added   []postgres.Table `json:"added"`
	Removed []postgres.Table `json:"removed"`
	Changed []TableChange    `json:"changed"`
}

type TableChange struct {
	Key           string         `json:"key"`
	Before        postgres.Table `json:"before"`
	After         postgres.Table `json:"after"`
	ChangedFields []string       `json:"changed_fields"`
}

type ObjectChange[T any] struct {
	Key           string   `json:"key"`
	Before        T        `json:"before"`
	After         T        `json:"after"`
	ChangedFields []string `json:"changed_fields"`
}

type ColumnDiff struct {
	Added   []postgres.Column               `json:"added"`
	Removed []postgres.Column               `json:"removed"`
	Changed []ObjectChange[postgres.Column] `json:"changed"`
}

type IndexDiff struct {
	Added   []postgres.Index               `json:"added"`
	Removed []postgres.Index               `json:"removed"`
	Changed []ObjectChange[postgres.Index] `json:"changed"`
}

type ConstraintDiff struct {
	Added   []postgres.Constraint               `json:"added"`
	Removed []postgres.Constraint               `json:"removed"`
	Changed []ObjectChange[postgres.Constraint] `json:"changed"`
}

type PolicyDiff struct {
	Added   []postgres.Policy `json:"added"`
	Removed []postgres.Policy `json:"removed"`
	Changed []PolicyChange    `json:"changed"`
}

type PolicyChange struct {
	Key           string          `json:"key"`
	Before        postgres.Policy `json:"before"`
	After         postgres.Policy `json:"after"`
	ChangedFields []string        `json:"changed_fields"`
}

type FunctionDiff struct {
	Added   []postgres.Function               `json:"added"`
	Removed []postgres.Function               `json:"removed"`
	Changed []ObjectChange[postgres.Function] `json:"changed"`
}

type TriggerDiff struct {
	Added   []postgres.Trigger               `json:"added"`
	Removed []postgres.Trigger               `json:"removed"`
	Changed []ObjectChange[postgres.Trigger] `json:"changed"`
}

type ExtensionDiff struct {
	Added   []postgres.Extension               `json:"added"`
	Removed []postgres.Extension               `json:"removed"`
	Changed []ObjectChange[postgres.Extension] `json:"changed"`
}

type StorageBucketDiff struct {
	Added   []postgres.StorageBucket `json:"added"`
	Removed []postgres.StorageBucket `json:"removed"`
	Changed []StorageBucketChange    `json:"changed"`
}

type StorageBucketChange struct {
	Key           string                 `json:"key"`
	Before        postgres.StorageBucket `json:"before"`
	After         postgres.StorageBucket `json:"after"`
	ChangedFields []string               `json:"changed_fields"`
}

type TableRowCountDiff struct {
	Added   []TableRowCount       `json:"added"`
	Removed []TableRowCount       `json:"removed"`
	Changed []TableRowCountChange `json:"changed"`
}

type TableRowCount struct {
	Key       string `json:"key"`
	Schema    string `json:"schema"`
	TableName string `json:"table_name"`
	Rows      int64  `json:"rows"`
}

type TableRowCountChange struct {
	Key    string        `json:"key"`
	Before TableRowCount `json:"before"`
	After  TableRowCount `json:"after"`
}

func (s *Store) Diff(ctx context.Context, baseline *Store) (DiffResult, error) {
	currentRef, err := s.archiveRef(ctx)
	if err != nil {
		return DiffResult{}, err
	}
	baselineRef, err := baseline.archiveRef(ctx)
	if err != nil {
		return DiffResult{}, err
	}

	currentTables, err := s.loadTables(ctx)
	if err != nil {
		return DiffResult{}, err
	}
	baselineTables, err := baseline.loadTables(ctx)
	if err != nil {
		return DiffResult{}, err
	}
	currentColumns, err := s.loadColumns(ctx)
	if err != nil {
		return DiffResult{}, err
	}
	baselineColumns, err := baseline.loadColumns(ctx)
	if err != nil {
		return DiffResult{}, err
	}
	currentIndexes, err := s.loadIndexes(ctx)
	if err != nil {
		return DiffResult{}, err
	}
	baselineIndexes, err := baseline.loadIndexes(ctx)
	if err != nil {
		return DiffResult{}, err
	}
	currentConstraints, err := s.loadConstraints(ctx)
	if err != nil {
		return DiffResult{}, err
	}
	baselineConstraints, err := baseline.loadConstraints(ctx)
	if err != nil {
		return DiffResult{}, err
	}
	currentPolicies, err := s.loadPolicies(ctx)
	if err != nil {
		return DiffResult{}, err
	}
	baselinePolicies, err := baseline.loadPolicies(ctx)
	if err != nil {
		return DiffResult{}, err
	}
	currentFunctions, err := s.loadFunctions(ctx)
	if err != nil {
		return DiffResult{}, err
	}
	baselineFunctions, err := baseline.loadFunctions(ctx)
	if err != nil {
		return DiffResult{}, err
	}
	currentTriggers, err := s.loadTriggers(ctx)
	if err != nil {
		return DiffResult{}, err
	}
	baselineTriggers, err := baseline.loadTriggers(ctx)
	if err != nil {
		return DiffResult{}, err
	}
	currentExtensions, err := s.loadExtensions(ctx)
	if err != nil {
		return DiffResult{}, err
	}
	baselineExtensions, err := baseline.loadExtensions(ctx)
	if err != nil {
		return DiffResult{}, err
	}
	currentBuckets, err := s.loadStorageBuckets(ctx)
	if err != nil {
		return DiffResult{}, err
	}
	baselineBuckets, err := baseline.loadStorageBuckets(ctx)
	if err != nil {
		return DiffResult{}, err
	}
	currentRowCounts, err := s.loadTableRowCounts(ctx)
	if err != nil {
		return DiffResult{}, err
	}
	baselineRowCounts, err := baseline.loadTableRowCounts(ctx)
	if err != nil {
		return DiffResult{}, err
	}

	result := DiffResult{
		Current:         currentRef,
		Baseline:        baselineRef,
		ProjectMismatch: currentRef.ProjectID != baselineRef.ProjectID,
		Tables:          diffTables(currentTables, baselineTables),
		Columns:         diffColumns(currentColumns, baselineColumns),
		Indexes:         diffIndexes(currentIndexes, baselineIndexes),
		Constraints:     diffConstraints(currentConstraints, baselineConstraints),
		Policies:        diffPolicies(currentPolicies, baselinePolicies),
		Functions:       diffFunctions(currentFunctions, baselineFunctions),
		Triggers:        diffTriggers(currentTriggers, baselineTriggers),
		Extensions:      diffExtensions(currentExtensions, baselineExtensions),
		StorageBuckets:  diffStorageBuckets(currentBuckets, baselineBuckets),
		TableRows:       diffTableRowCounts(currentRowCounts, baselineRowCounts),
	}
	return result, nil
}

func (s *Store) archiveRef(ctx context.Context) (ArchiveRef, error) {
	ref := ArchiveRef{Path: s.path}
	var collectedAt string
	err := s.db.QueryRowContext(ctx, `select project_id, collected_at from project_info where id = 'default'`).Scan(&ref.ProjectID, &collectedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ref, nil
		}
		return ArchiveRef{}, err
	}
	if collectedAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, collectedAt)
		if err == nil {
			ref.CollectedAt = parsed
		}
	}
	return ref, nil
}

func (s *Store) loadTables(ctx context.Context) ([]postgres.Table, error) {
	rows, err := s.db.QueryContext(ctx, `
select schema_name, name, kind, owner, comment, rls_enabled, rls_forced, estimated_rows
from tables
order by schema_name, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []postgres.Table
	for rows.Next() {
		var row postgres.Table
		var rlsEnabled, rlsForced int
		if err := rows.Scan(&row.Schema, &row.Name, &row.Kind, &row.Owner, &row.Comment, &rlsEnabled, &rlsForced, &row.EstimatedRows); err != nil {
			return nil, err
		}
		row.RLSEnabled = intBool(rlsEnabled)
		row.RLSForced = intBool(rlsForced)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) loadColumns(ctx context.Context) ([]postgres.Column, error) {
	rows, err := s.db.QueryContext(ctx, `
select table_schema, table_name, name, ordinal, data_type, is_nullable, default_value, comment
from columns
order by table_schema, table_name, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []postgres.Column
	for rows.Next() {
		var row postgres.Column
		var isNullable int
		if err := rows.Scan(&row.TableSchema, &row.TableName, &row.Name, &row.Ordinal, &row.DataType, &isNullable, &row.Default, &row.Comment); err != nil {
			return nil, err
		}
		row.IsNullable = intBool(isNullable)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) loadIndexes(ctx context.Context) ([]postgres.Index, error) {
	rows, err := s.db.QueryContext(ctx, `
select schema_name, table_name, name, definition
from indexes
order by schema_name, table_name, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []postgres.Index
	for rows.Next() {
		var row postgres.Index
		if err := rows.Scan(&row.Schema, &row.TableName, &row.Name, &row.Definition); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) loadConstraints(ctx context.Context) ([]postgres.Constraint, error) {
	rows, err := s.db.QueryContext(ctx, `
select schema_name, table_name, name, type, definition
from constraints
order by schema_name, table_name, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []postgres.Constraint
	for rows.Next() {
		var row postgres.Constraint
		if err := rows.Scan(&row.Schema, &row.TableName, &row.Name, &row.Type, &row.Definition); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) loadPolicies(ctx context.Context) ([]postgres.Policy, error) {
	rows, err := s.db.QueryContext(ctx, `
select schema_name, table_name, name, command, roles, using_expr, check_expr
from policies
order by schema_name, table_name, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []postgres.Policy
	for rows.Next() {
		var row postgres.Policy
		if err := rows.Scan(&row.Schema, &row.TableName, &row.Name, &row.Command, &row.Roles, &row.Using, &row.Check); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) loadFunctions(ctx context.Context) ([]postgres.Function, error) {
	rows, err := s.db.QueryContext(ctx, `
select schema_name, name, identity_args, returns, language, security_definer, definition
from functions
order by schema_name, name, identity_args`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []postgres.Function
	for rows.Next() {
		var row postgres.Function
		var securityDefiner int
		if err := rows.Scan(&row.Schema, &row.Name, &row.IdentityArgs, &row.Returns, &row.Language, &securityDefiner, &row.Definition); err != nil {
			return nil, err
		}
		row.SecurityDefiner = intBool(securityDefiner)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) loadTriggers(ctx context.Context) ([]postgres.Trigger, error) {
	rows, err := s.db.QueryContext(ctx, `
select schema_name, table_name, name, timing, events, function_name, definition
from triggers
order by schema_name, table_name, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []postgres.Trigger
	for rows.Next() {
		var row postgres.Trigger
		if err := rows.Scan(&row.Schema, &row.TableName, &row.Name, &row.Timing, &row.Events, &row.FunctionName, &row.Definition); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) loadExtensions(ctx context.Context) ([]postgres.Extension, error) {
	rows, err := s.db.QueryContext(ctx, `
select name, schema_name, version, comment
from extensions
order by name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []postgres.Extension
	for rows.Next() {
		var row postgres.Extension
		if err := rows.Scan(&row.Name, &row.Schema, &row.Version, &row.Comment); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) loadStorageBuckets(ctx context.Context) ([]postgres.StorageBucket, error) {
	rows, err := s.db.QueryContext(ctx, `
select id, name, public, file_size_limit, allowed_mime_types, created_at, updated_bucket_at
from storage_buckets
order by id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []postgres.StorageBucket
	for rows.Next() {
		var row postgres.StorageBucket
		var public int
		if err := rows.Scan(&row.ID, &row.Name, &public, &row.FileSizeLimit, &row.AllowedMimeTypes, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, err
		}
		row.Public = intBool(public)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) loadTableRowCounts(ctx context.Context) ([]TableRowCount, error) {
	rows, err := s.db.QueryContext(ctx, `
select schema_name, table_name, count(*)
from table_rows
group by schema_name, table_name
order by schema_name, table_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TableRowCount
	for rows.Next() {
		var row TableRowCount
		if err := rows.Scan(&row.Schema, &row.TableName, &row.Rows); err != nil {
			return nil, err
		}
		row.Key = tableRowCountKey(row)
		out = append(out, row)
	}
	return out, rows.Err()
}

func diffObjects[T any](current, baseline []T, key func(T) string, changedFields func(T, T) []string) ([]T, []T, []ObjectChange[T]) {
	added := make([]T, 0)
	removed := make([]T, 0)
	changed := make([]ObjectChange[T], 0)
	currentByKey := make(map[string]T, len(current))
	for _, row := range current {
		currentByKey[key(row)] = row
	}
	baselineByKey := make(map[string]T, len(baseline))
	for _, row := range baseline {
		baselineByKey[key(row)] = row
	}

	for _, row := range current {
		rowKey := key(row)
		before, ok := baselineByKey[rowKey]
		if !ok {
			added = append(added, row)
			continue
		}
		fields := changedFields(before, row)
		if len(fields) > 0 {
			changed = append(changed, ObjectChange[T]{Key: rowKey, Before: before, After: row, ChangedFields: fields})
		}
	}
	for _, row := range baseline {
		if _, ok := currentByKey[key(row)]; !ok {
			removed = append(removed, row)
		}
	}

	sort.Slice(added, func(i, j int) bool { return key(added[i]) < key(added[j]) })
	sort.Slice(removed, func(i, j int) bool { return key(removed[i]) < key(removed[j]) })
	sort.Slice(changed, func(i, j int) bool { return changed[i].Key < changed[j].Key })
	return added, removed, changed
}

func diffTables(current, baseline []postgres.Table) TableDiff {
	diff := TableDiff{
		Added:   make([]postgres.Table, 0),
		Removed: make([]postgres.Table, 0),
		Changed: make([]TableChange, 0),
	}
	currentByKey := make(map[string]postgres.Table, len(current))
	for _, row := range current {
		currentByKey[tableKey(row)] = row
	}
	baselineByKey := make(map[string]postgres.Table, len(baseline))
	for _, row := range baseline {
		baselineByKey[tableKey(row)] = row
	}

	for _, row := range current {
		key := tableKey(row)
		before, ok := baselineByKey[key]
		if !ok {
			diff.Added = append(diff.Added, row)
			continue
		}
		fields := changedTableFields(before, row)
		if len(fields) > 0 {
			diff.Changed = append(diff.Changed, TableChange{Key: key, Before: before, After: row, ChangedFields: fields})
		}
	}
	for _, row := range baseline {
		if _, ok := currentByKey[tableKey(row)]; !ok {
			diff.Removed = append(diff.Removed, row)
		}
	}

	sort.Slice(diff.Added, func(i, j int) bool { return tableKey(diff.Added[i]) < tableKey(diff.Added[j]) })
	sort.Slice(diff.Removed, func(i, j int) bool { return tableKey(diff.Removed[i]) < tableKey(diff.Removed[j]) })
	sort.Slice(diff.Changed, func(i, j int) bool { return diff.Changed[i].Key < diff.Changed[j].Key })
	return diff
}

func diffColumns(current, baseline []postgres.Column) ColumnDiff {
	added, removed, changed := diffObjects(current, baseline, columnKey, changedColumnFields)
	return ColumnDiff{Added: added, Removed: removed, Changed: changed}
}

func diffIndexes(current, baseline []postgres.Index) IndexDiff {
	added, removed, changed := diffObjects(current, baseline, indexKey, changedIndexFields)
	return IndexDiff{Added: added, Removed: removed, Changed: changed}
}

func diffConstraints(current, baseline []postgres.Constraint) ConstraintDiff {
	added, removed, changed := diffObjects(current, baseline, constraintKey, changedConstraintFields)
	return ConstraintDiff{Added: added, Removed: removed, Changed: changed}
}

func diffPolicies(current, baseline []postgres.Policy) PolicyDiff {
	diff := PolicyDiff{
		Added:   make([]postgres.Policy, 0),
		Removed: make([]postgres.Policy, 0),
		Changed: make([]PolicyChange, 0),
	}
	currentByKey := make(map[string]postgres.Policy, len(current))
	for _, row := range current {
		currentByKey[policyKey(row)] = row
	}
	baselineByKey := make(map[string]postgres.Policy, len(baseline))
	for _, row := range baseline {
		baselineByKey[policyKey(row)] = row
	}

	for _, row := range current {
		key := policyKey(row)
		before, ok := baselineByKey[key]
		if !ok {
			diff.Added = append(diff.Added, row)
			continue
		}
		fields := changedPolicyFields(before, row)
		if len(fields) > 0 {
			diff.Changed = append(diff.Changed, PolicyChange{Key: key, Before: before, After: row, ChangedFields: fields})
		}
	}
	for _, row := range baseline {
		if _, ok := currentByKey[policyKey(row)]; !ok {
			diff.Removed = append(diff.Removed, row)
		}
	}

	sort.Slice(diff.Added, func(i, j int) bool { return policyKey(diff.Added[i]) < policyKey(diff.Added[j]) })
	sort.Slice(diff.Removed, func(i, j int) bool { return policyKey(diff.Removed[i]) < policyKey(diff.Removed[j]) })
	sort.Slice(diff.Changed, func(i, j int) bool { return diff.Changed[i].Key < diff.Changed[j].Key })
	return diff
}

func diffFunctions(current, baseline []postgres.Function) FunctionDiff {
	added, removed, changed := diffObjects(current, baseline, functionKey, changedFunctionFields)
	return FunctionDiff{Added: added, Removed: removed, Changed: changed}
}

func diffTriggers(current, baseline []postgres.Trigger) TriggerDiff {
	added, removed, changed := diffObjects(current, baseline, triggerKey, changedTriggerFields)
	return TriggerDiff{Added: added, Removed: removed, Changed: changed}
}

func diffExtensions(current, baseline []postgres.Extension) ExtensionDiff {
	added, removed, changed := diffObjects(current, baseline, extensionKey, changedExtensionFields)
	return ExtensionDiff{Added: added, Removed: removed, Changed: changed}
}

func diffStorageBuckets(current, baseline []postgres.StorageBucket) StorageBucketDiff {
	diff := StorageBucketDiff{
		Added:   make([]postgres.StorageBucket, 0),
		Removed: make([]postgres.StorageBucket, 0),
		Changed: make([]StorageBucketChange, 0),
	}
	currentByKey := make(map[string]postgres.StorageBucket, len(current))
	for _, row := range current {
		currentByKey[storageBucketKey(row)] = row
	}
	baselineByKey := make(map[string]postgres.StorageBucket, len(baseline))
	for _, row := range baseline {
		baselineByKey[storageBucketKey(row)] = row
	}

	for _, row := range current {
		key := storageBucketKey(row)
		before, ok := baselineByKey[key]
		if !ok {
			diff.Added = append(diff.Added, row)
			continue
		}
		fields := changedStorageBucketFields(before, row)
		if len(fields) > 0 {
			diff.Changed = append(diff.Changed, StorageBucketChange{Key: key, Before: before, After: row, ChangedFields: fields})
		}
	}
	for _, row := range baseline {
		if _, ok := currentByKey[storageBucketKey(row)]; !ok {
			diff.Removed = append(diff.Removed, row)
		}
	}

	sort.Slice(diff.Added, func(i, j int) bool { return storageBucketKey(diff.Added[i]) < storageBucketKey(diff.Added[j]) })
	sort.Slice(diff.Removed, func(i, j int) bool { return storageBucketKey(diff.Removed[i]) < storageBucketKey(diff.Removed[j]) })
	sort.Slice(diff.Changed, func(i, j int) bool { return diff.Changed[i].Key < diff.Changed[j].Key })
	return diff
}

func diffTableRowCounts(current, baseline []TableRowCount) TableRowCountDiff {
	diff := TableRowCountDiff{
		Added:   make([]TableRowCount, 0),
		Removed: make([]TableRowCount, 0),
		Changed: make([]TableRowCountChange, 0),
	}
	currentByKey := make(map[string]TableRowCount, len(current))
	for _, row := range current {
		currentByKey[row.Key] = row
	}
	baselineByKey := make(map[string]TableRowCount, len(baseline))
	for _, row := range baseline {
		baselineByKey[row.Key] = row
	}

	for _, row := range current {
		before, ok := baselineByKey[row.Key]
		if !ok {
			diff.Added = append(diff.Added, row)
			continue
		}
		if before.Rows != row.Rows {
			diff.Changed = append(diff.Changed, TableRowCountChange{Key: row.Key, Before: before, After: row})
		}
	}
	for _, row := range baseline {
		if _, ok := currentByKey[row.Key]; !ok {
			diff.Removed = append(diff.Removed, row)
		}
	}

	sort.Slice(diff.Added, func(i, j int) bool { return diff.Added[i].Key < diff.Added[j].Key })
	sort.Slice(diff.Removed, func(i, j int) bool { return diff.Removed[i].Key < diff.Removed[j].Key })
	sort.Slice(diff.Changed, func(i, j int) bool { return diff.Changed[i].Key < diff.Changed[j].Key })
	return diff
}

func changedTableFields(before, after postgres.Table) []string {
	fields := make([]string, 0)
	if before.RLSEnabled != after.RLSEnabled {
		fields = append(fields, "rls_enabled")
	}
	if before.RLSForced != after.RLSForced {
		fields = append(fields, "rls_forced")
	}
	if before.Comment != after.Comment {
		fields = append(fields, "comment")
	}
	if before.Kind != after.Kind {
		fields = append(fields, "kind")
	}
	return sortedFields(fields)
}

func changedColumnFields(before, after postgres.Column) []string {
	fields := make([]string, 0)
	if before.Ordinal != after.Ordinal {
		fields = append(fields, "ordinal")
	}
	if before.DataType != after.DataType {
		fields = append(fields, "data_type")
	}
	if before.IsNullable != after.IsNullable {
		fields = append(fields, "is_nullable")
	}
	if before.Default != after.Default {
		fields = append(fields, "default")
	}
	if before.Comment != after.Comment {
		fields = append(fields, "comment")
	}
	return sortedFields(fields)
}

func changedIndexFields(before, after postgres.Index) []string {
	fields := make([]string, 0)
	if before.Definition != after.Definition {
		fields = append(fields, "definition")
	}
	return sortedFields(fields)
}

func changedConstraintFields(before, after postgres.Constraint) []string {
	fields := make([]string, 0)
	if before.Type != after.Type {
		fields = append(fields, "type")
	}
	if before.Definition != after.Definition {
		fields = append(fields, "definition")
	}
	return sortedFields(fields)
}

func changedPolicyFields(before, after postgres.Policy) []string {
	fields := make([]string, 0)
	if before.Command != after.Command {
		fields = append(fields, "command")
	}
	if before.Roles != after.Roles {
		fields = append(fields, "roles")
	}
	if before.Using != after.Using {
		fields = append(fields, "using")
	}
	if before.Check != after.Check {
		fields = append(fields, "check")
	}
	return sortedFields(fields)
}

func changedFunctionFields(before, after postgres.Function) []string {
	fields := make([]string, 0)
	if before.Returns != after.Returns {
		fields = append(fields, "returns")
	}
	if before.Language != after.Language {
		fields = append(fields, "language")
	}
	if before.SecurityDefiner != after.SecurityDefiner {
		fields = append(fields, "security_definer")
	}
	if before.Definition != after.Definition {
		fields = append(fields, "definition")
	}
	return sortedFields(fields)
}

func changedTriggerFields(before, after postgres.Trigger) []string {
	fields := make([]string, 0)
	if before.Timing != after.Timing {
		fields = append(fields, "timing")
	}
	if before.Events != after.Events {
		fields = append(fields, "events")
	}
	if before.FunctionName != after.FunctionName {
		fields = append(fields, "function_name")
	}
	if before.Definition != after.Definition {
		fields = append(fields, "definition")
	}
	return sortedFields(fields)
}

func changedExtensionFields(before, after postgres.Extension) []string {
	fields := make([]string, 0)
	if before.Schema != after.Schema {
		fields = append(fields, "schema")
	}
	if before.Version != after.Version {
		fields = append(fields, "version")
	}
	if before.Comment != after.Comment {
		fields = append(fields, "comment")
	}
	return sortedFields(fields)
}

func changedStorageBucketFields(before, after postgres.StorageBucket) []string {
	fields := make([]string, 0)
	if before.Public != after.Public {
		fields = append(fields, "public")
	}
	if before.FileSizeLimit != after.FileSizeLimit {
		fields = append(fields, "file_size_limit")
	}
	if before.AllowedMimeTypes != after.AllowedMimeTypes {
		fields = append(fields, "allowed_mime_types")
	}
	return sortedFields(fields)
}

func tableKey(row postgres.Table) string {
	return row.Schema + "." + row.Name
}

func columnKey(row postgres.Column) string {
	return row.TableSchema + "." + row.TableName + "." + row.Name
}

func indexKey(row postgres.Index) string {
	return row.Schema + "." + row.TableName + "." + row.Name
}

func constraintKey(row postgres.Constraint) string {
	return row.Schema + "." + row.TableName + "." + row.Name
}

func policyKey(row postgres.Policy) string {
	return row.Schema + "." + row.TableName + "." + row.Name
}

func functionKey(row postgres.Function) string {
	return row.Schema + "." + row.Name + "(" + row.IdentityArgs + ")"
}

func triggerKey(row postgres.Trigger) string {
	return row.Schema + "." + row.TableName + "." + row.Name
}

func extensionKey(row postgres.Extension) string {
	return row.Name
}

func storageBucketKey(row postgres.StorageBucket) string {
	return row.ID
}

func tableRowCountKey(row TableRowCount) string {
	return row.Schema + "." + row.TableName
}

func sortedFields(fields []string) []string {
	sort.Strings(fields)
	return fields
}

func intBool(value int) bool {
	return value != 0
}
