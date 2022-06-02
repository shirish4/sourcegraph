package jobutil

import (
	"github.com/sourcegraph/sourcegraph/internal/search/job"
	"github.com/sourcegraph/sourcegraph/internal/search/query"
	"github.com/sourcegraph/sourcegraph/internal/search/run"
)

// NewFeelingLuckySearchJob generates an opportunistic search query by applying
// various rules in sequence, transforming the original input plan into various
// queries that alter its interpretation (e.g., search literally for quotes or
// not, attempt to search the pattern as a regexp, and so on). Generated queries
// are appended to the input plan, such that the resulting job has the following
// properties:
//
// - Every basic query in the input plan will be run (ordered before) any
// generated, opportunistic query. This means there is a kind of ranking that
// prefers the most precise interpretation of inputs first (and its results),
// and only after that starts to generate queries with looser or alternative
// interpretations (these interpretations are determined by the application
// order of rules.
//
// - The application order of rules is deterministic. This means the query
// order, and therefore runtime execution, outputs the same search results for
// the same inputs. I.e., there is no random choice when applying a rule.
func NewFeelingLuckySearchJob(inputs *run.SearchInputs, plan query.Plan) job.Job {
	children := make([]job.Job, 0, len(plan))

	// Sequence all basic queries in the plan to run first.
	for _, b := range plan {
		child, err := NewBasicJob(inputs, b)
		if err != nil {
			panic("TODO")
		}
		children = append(children, child)
	}

	// Sequence all generated queries afterward the first.
	for _, b := range plan {
		for _, newBasic := range applyRulesList(b, rulesList...) {
			child, err := NewBasicJob(inputs, newBasic)
			if err != nil {
				panic("generated an invalid basic query D:")
			}
			children = append(children, child)
		}
	}
	return NewSequentialJob(true, children...)
}

var rulesList = [][]rule{
	[]rule{unquotePatterns},
}

// rule represents a transformation function on a Basic query. Applying rules
// cannot fail: either they apply and produce a valid, non-nil, Basic query, or
// they cannot apply, in which case they return nil. See the `unquotePatterns`
// rule for an example.
type rule func(query.Basic) *query.Basic

// applyRulesList takes a list of lists of rules. The order of rules in the inner
// lists represent rule composition. Each list of rules in the outer list
// represent one possible query production, if the sequence of the rules in this
// list apply successfully. Example:
//
// If we have input rule list  [ [ R1, R2 ], [ R2 ] ] and input query B0, then:
//
// - If both inner lists apply, we get an output [ B1, B2] where B1 is generated
//   from applying R1 then R2, and B2 is generated from just applying R2.
// - If only the first inner list applies, R1 then R2, we get the output [ B1 ]
// - If only the second inner list applies, R2 on its own, we get the output [ B2 ]
func applyRulesList(b query.Basic, rulesList ...[]rule) []query.Basic {
	bs := []query.Basic{}
	for _, l := range rulesList {
		if generated := applyRules(b, l...); generated != nil {
			bs = append(bs, *generated)
		}
	}
	return bs
}

// applyRules applies every rule in sequence to `b`. If any rule does not apply, it returns nil.
func applyRules(b query.Basic, rules ...rule) *query.Basic {
	if len(rules) == 0 {
		return &b
	}
	if next := rules[0](b); next != nil {
		return applyRules(*next, rules[1:]...)
	}
	return nil
}

// unquotePatterns is a rule that unquotes all patterns in the input query (it
// removes quotes, and honors escape sequences inside quoted values).
func unquotePatterns(b query.Basic) *query.Basic {
	// Go back all the way to the raw tree representation :-). We just parse
	// the string as regex, since parsing with regex annotates quoted
	// patterns.
	rawParseTree, err := query.Parse(query.StringHuman(b.ToParseTree()), query.SearchTypeRegex)
	if err != nil {
		return nil
	}

	changed := false // track whether we've successfully changed any pattern, which means this rule applies.
	newParseTree := query.MapPattern(rawParseTree, func(value string, negated bool, annotation query.Annotation) query.Node {
		if annotation.Labels.IsSet(query.Quoted) {
			changed = true
			annotation.Labels.Unset(query.Quoted)
			annotation.Labels.Set(query.Literal)
			return query.Pattern{
				Value:      value,
				Negated:    negated,
				Annotation: annotation,
			}
		}
		return query.Pattern{
			Value:      value,
			Negated:    negated,
			Annotation: annotation,
		}
	})

	if !changed {
		// No unquoting happened, so we don't run the search.
		return nil
	}

	newNodes, err := query.Sequence(query.For(query.SearchTypeLiteralDefault))(newParseTree)
	if err != nil {
		return nil
	}

	newBasic, err := query.ToBasicQuery(newNodes)
	if err != nil {
		return nil
	}

	return &newBasic
}
