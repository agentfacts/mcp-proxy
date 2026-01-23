package compiler

import (
	"fmt"
	"regexp"
	"strings"
)

// ExpressionCompiler compiles JSON expressions to Rego code.
type ExpressionCompiler struct {
	indent int
}

// NewExpressionCompiler creates a new expression compiler.
func NewExpressionCompiler() *ExpressionCompiler {
	return &ExpressionCompiler{indent: 1}
}

// Compile compiles a condition expression to Rego code.
func (ec *ExpressionCompiler) Compile(expr map[string]interface{}) (string, error) {
	return ec.compileExpr(expr, ec.indent)
}

func (ec *ExpressionCompiler) compileExpr(expr map[string]interface{}, indent int) (string, error) {
	indentStr := strings.Repeat("    ", indent)

	// Check for logical operators first
	if all, ok := expr["all"]; ok {
		return ec.compileAll(all, indent)
	}

	if any, ok := expr["any"]; ok {
		return ec.compileAny(any, indent)
	}

	if not, ok := expr["not"]; ok {
		return ec.compileNot(not, indent)
	}

	// Shorthand conditions
	if toolIn, ok := expr["tool_in"]; ok {
		values, err := toStringSlice(toolIn)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%sinput.request.tool in %s", indentStr, quoteSlice(values)), nil
	}

	if agentIn, ok := expr["agent_in"]; ok {
		values, err := toStringSlice(agentIn)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%sinput.agent.id in %s", indentStr, quoteSlice(values)), nil
	}

	if fieldEquals, ok := expr["field_equals"]; ok {
		return ec.compileFieldEquals(fieldEquals, indent)
	}

	if fieldIn, ok := expr["field_in"]; ok {
		return ec.compileFieldIn(fieldIn, indent)
	}

	if fieldMatches, ok := expr["field_matches"]; ok {
		return ec.compileFieldMatches(fieldMatches, indent)
	}

	// Field operation with explicit operator
	if field, ok := expr["field"]; ok {
		return ec.compileFieldOp(field.(string), expr, indent)
	}

	// Check for direct field path comparisons (e.g., {"identity.verified": true})
	for key, value := range expr {
		if isFieldPath(key) {
			return ec.compileDirectComparison(key, value, indent)
		}
	}

	return "", fmt.Errorf("unknown expression type: %v", expr)
}

func (ec *ExpressionCompiler) compileAll(all interface{}, indent int) (string, error) {
	items, ok := all.([]interface{})
	if !ok {
		return "", fmt.Errorf("'all' must be an array")
	}

	var conditions []string
	for i, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("all[%d] must be an object", i)
		}
		cond, err := ec.compileExpr(itemMap, indent)
		if err != nil {
			return "", fmt.Errorf("all[%d]: %w", i, err)
		}
		conditions = append(conditions, cond)
	}

	return strings.Join(conditions, "\n"), nil
}

func (ec *ExpressionCompiler) compileAny(any interface{}, indent int) (string, error) {
	items, ok := any.([]interface{})
	if !ok {
		return "", fmt.Errorf("'any' must be an array")
	}

	indentStr := strings.Repeat("    ", indent)
	var conditions []string
	for i, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("any[%d] must be an object", i)
		}
		cond, err := ec.compileExpr(itemMap, indent+1)
		if err != nil {
			return "", fmt.Errorf("any[%d]: %w", i, err)
		}
		conditions = append(conditions, cond)
	}

	// OPA any: use helper rule or comprehension
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("%s# any of the following\n", indentStr))
	builder.WriteString(fmt.Sprintf("%s{\n", indentStr))
	for _, cond := range conditions {
		builder.WriteString(fmt.Sprintf("%s    %s\n", indentStr, strings.TrimSpace(cond)))
	}
	builder.WriteString(fmt.Sprintf("%s} | {\n", indentStr))
	// Close with alternative syntax for OPA
	builder.WriteString(fmt.Sprintf("%s    true\n", indentStr))
	builder.WriteString(fmt.Sprintf("%s}", indentStr))

	// Actually, let's use a simpler approach with helper rules
	// For now, just OR them together
	var orConditions []string
	for _, cond := range conditions {
		orConditions = append(orConditions, fmt.Sprintf("(%s)", strings.TrimSpace(cond)))
	}

	return fmt.Sprintf("%s# any: one of the following must be true\n%s%s",
		indentStr, indentStr, strings.Join(orConditions, fmt.Sprintf("\n%s", indentStr))), nil
}

func (ec *ExpressionCompiler) compileNot(not interface{}, indent int) (string, error) {
	notMap, ok := not.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("'not' must be an object")
	}

	indentStr := strings.Repeat("    ", indent)
	inner, err := ec.compileExpr(notMap, 0)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%snot %s", indentStr, strings.TrimSpace(inner)), nil
}

func (ec *ExpressionCompiler) compileFieldEquals(fieldEquals interface{}, indent int) (string, error) {
	indentStr := strings.Repeat("    ", indent)
	feMap, ok := fieldEquals.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("'field_equals' must be an object")
	}

	var conditions []string
	for path, value := range feMap {
		regoPath := fieldPathToRego(path)
		regoValue := valueToRego(value)
		conditions = append(conditions, fmt.Sprintf("%s%s == %s", indentStr, regoPath, regoValue))
	}

	return strings.Join(conditions, "\n"), nil
}

func (ec *ExpressionCompiler) compileFieldIn(fieldIn interface{}, indent int) (string, error) {
	indentStr := strings.Repeat("    ", indent)
	fiMap, ok := fieldIn.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("'field_in' must be an object")
	}

	var conditions []string
	for path, valuesRaw := range fiMap {
		values, err := toStringSlice(valuesRaw)
		if err != nil {
			return "", err
		}
		regoPath := fieldPathToRego(path)
		conditions = append(conditions, fmt.Sprintf("%s%s in %s", indentStr, regoPath, quoteSlice(values)))
	}

	return strings.Join(conditions, "\n"), nil
}

func (ec *ExpressionCompiler) compileFieldMatches(fieldMatches interface{}, indent int) (string, error) {
	indentStr := strings.Repeat("    ", indent)
	fmMap, ok := fieldMatches.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("'field_matches' must be an object")
	}

	var conditions []string
	for path, pattern := range fmMap {
		patternStr, ok := pattern.(string)
		if !ok {
			return "", fmt.Errorf("pattern for '%s' must be a string", path)
		}
		regoPath := fieldPathToRego(path)
		conditions = append(conditions, fmt.Sprintf("%sregex.match(%q, %s)", indentStr, patternStr, regoPath))
	}

	return strings.Join(conditions, "\n"), nil
}

func (ec *ExpressionCompiler) compileFieldOp(field string, expr map[string]interface{}, indent int) (string, error) {
	indentStr := strings.Repeat("    ", indent)
	regoPath := fieldPathToRego(field)

	op, ok := expr["op"].(string)
	if !ok {
		return "", fmt.Errorf("'op' is required for field operations")
	}

	value := expr["value"]

	switch Operator(op) {
	case OpEquals:
		return fmt.Sprintf("%s%s == %s", indentStr, regoPath, valueToRego(value)), nil
	case OpNotEquals:
		return fmt.Sprintf("%s%s != %s", indentStr, regoPath, valueToRego(value)), nil
	case OpGreaterThan:
		return fmt.Sprintf("%s%s > %s", indentStr, regoPath, valueToRego(value)), nil
	case OpGreaterEq:
		return fmt.Sprintf("%s%s >= %s", indentStr, regoPath, valueToRego(value)), nil
	case OpLessThan:
		return fmt.Sprintf("%s%s < %s", indentStr, regoPath, valueToRego(value)), nil
	case OpLessEq:
		return fmt.Sprintf("%s%s <= %s", indentStr, regoPath, valueToRego(value)), nil
	case OpContains:
		return fmt.Sprintf("%scontains(%s, %s)", indentStr, regoPath, valueToRego(value)), nil
	case OpStartsWith:
		return fmt.Sprintf("%sstartswith(%s, %s)", indentStr, regoPath, valueToRego(value)), nil
	case OpEndsWith:
		return fmt.Sprintf("%sendswith(%s, %s)", indentStr, regoPath, valueToRego(value)), nil
	case OpMatches:
		return fmt.Sprintf("%sregex.match(%s, %s)", indentStr, valueToRego(value), regoPath), nil
	case OpIn:
		values, err := toStringSlice(value)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s%s in %s", indentStr, regoPath, quoteSlice(values)), nil
	case OpNotIn:
		values, err := toStringSlice(value)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%snot %s in %s", indentStr, regoPath, quoteSlice(values)), nil
	default:
		return "", fmt.Errorf("unknown operator: %s", op)
	}
}

func (ec *ExpressionCompiler) compileDirectComparison(path string, value interface{}, indent int) (string, error) {
	indentStr := strings.Repeat("    ", indent)
	regoPath := fieldPathToRego(path)
	return fmt.Sprintf("%s%s == %s", indentStr, regoPath, valueToRego(value)), nil
}

// fieldPathToRego converts a JSON field path to Rego input path.
// e.g., "agent.id" -> "input.agent.id"
// e.g., "request.arguments.user_id" -> "input.request.arguments.user_id"
func fieldPathToRego(path string) string {
	if strings.HasPrefix(path, "input.") {
		return path
	}
	return "input." + path
}

// valueToRego converts a Go value to Rego literal.
func valueToRego(v interface{}) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("%q", val)
	case bool:
		return fmt.Sprintf("%t", val)
	case int:
		return fmt.Sprintf("%d", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%f", val)
	case []interface{}:
		strs := make([]string, len(val))
		for i, item := range val {
			strs[i] = valueToRego(item)
		}
		return "[" + strings.Join(strs, ", ") + "]"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", val)
	}
}

// isFieldPath checks if a string looks like a field path (contains dots, alphanumeric).
func isFieldPath(s string) bool {
	// Skip known keywords
	keywords := map[string]bool{
		"all": true, "any": true, "not": true,
		"field": true, "op": true, "value": true,
		"field_equals": true, "field_in": true, "field_matches": true,
		"tool_in": true, "agent_in": true,
	}
	if keywords[s] {
		return false
	}

	matched, _ := regexp.MatchString(`^[a-zA-Z][a-zA-Z0-9_.]*$`, s)
	return matched
}
