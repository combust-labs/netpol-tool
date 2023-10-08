package tool

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func TestSelectors(t *testing.T) {

	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{
			"test-label": "value",
		},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			metav1.LabelSelectorRequirement{
				Key:      "test-matches-label",
				Operator: metav1.LabelSelectorOpExists,
				Values:   []string{},
			},
			metav1.LabelSelectorRequirement{
				Key:      "test-matches-label",
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{"a", "b"},
			},
		},
	})

	assert.Nil(t, err)

	t.Run("it=matches matchExpressions 'in' satisfied with a", func(tt *testing.T) {
		aSet := labels.Set(map[string]string{
			"test-label":         "value",
			"test-matches-label": "a",
		})
		assert.True(tt, selector.Matches(aSet))
		requirements, selectable := selector.Requirements()
		assert.True(tt, selectable)
		results := []bool{}
		for _, requirement := range requirements {
			results = append(results, requirement.Matches(aSet))
		}
		assert.Equal(tt, []bool{true, true, true}, results)
	})

	t.Run("it=matches matchExpressions 'in' satisfied with b", func(tt *testing.T) {
		aSet := labels.Set(map[string]string{
			"test-label":         "value",
			"test-matches-label": "b",
		})
		assert.True(tt, selector.Matches(aSet))
		requirements, selectable := selector.Requirements()
		assert.True(tt, selectable)
		results := []bool{}
		for _, requirement := range requirements {
			results = append(results, requirement.Matches(aSet))
		}
		assert.Equal(tt, []bool{true, true, true}, results)
	})

	t.Run("it=rejects both expressions when matchExpression 'in' violated", func(tt *testing.T) {
		aSet := labels.Set(map[string]string{
			"test-label":         "value",
			"test-matches-label": "c",
		})
		assert.False(tt, selector.Matches(aSet))
		requirements, selectable := selector.Requirements()
		assert.True(tt, selectable)
		results := []bool{}
		for _, requirement := range requirements {
			results = append(results, requirement.Matches(aSet))
		}
		assert.Equal(tt, []bool{true, true, false}, results)
	})

	t.Run("it=rejects both expressions when matchExpression 'exists' violated", func(tt *testing.T) {
		aSet := labels.Set(map[string]string{
			"test-label": "value",
		})
		assert.False(tt, selector.Matches(aSet))
		requirements, selectable := selector.Requirements()
		assert.True(tt, selectable)
		results := []bool{}
		for _, requirement := range requirements {
			results = append(results, requirement.Matches(aSet))
		}
		assert.Equal(tt, []bool{true, false, false}, results)
	})

	t.Run("it=rejects expression without a label", func(tt *testing.T) {
		aSet := labels.Set(map[string]string{
			"test-label": "value",
		})
		assert.False(tt, selector.Matches(aSet))
		requirements, selectable := selector.Requirements()
		assert.True(tt, selectable)
		results := []bool{}
		for _, requirement := range requirements {
			results = append(results, requirement.Matches(aSet))
		}
		assert.Equal(tt, []bool{true, false, false}, results)
	})

	t.Run("it=rejects all when labels don't match and matchExpressions unsatisfied", func(tt *testing.T) {
		aSet := labels.Set(map[string]string{
			"test-label": "incorrect",
		})
		assert.False(tt, selector.Matches(aSet))
		requirements, selectable := selector.Requirements()
		assert.True(tt, selectable)
		results := []bool{}
		for _, requirement := range requirements {
			results = append(results, requirement.Matches(aSet))
		}
		assert.Equal(tt, []bool{false, false, false}, results)
	})

}
