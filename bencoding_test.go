package bencode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMarshalAndUnmarshal(t *testing.T) {
	testCases := []struct {
		Name     string
		Value    any
		Encoding string
		Out      any
	}{
		{
			Name:     "string",
			Value:    "hello, world!",
			Encoding: "13:hello, world!",
			Out:      new(string),
		},
		{
			Name:     "zero integer",
			Value:    0,
			Encoding: "i0e",
			Out:      new(int),
		},
		{
			Name:     "positive integer",
			Value:    123,
			Encoding: "i123e",
			Out:      new(int),
		},
		{
			Name:     "negative integer",
			Value:    -123,
			Encoding: "i-123e",
			Out:      new(int),
		},
		// {
		// 	Name:     "list of strings",
		// 	Value:    []string{"1", "2", "3"},
		// 	Encoding: "l1:11:21:3e",
		// 	Out:      new([]string),
		// },
		// {
		// 	Name:     "list of ints",
		// 	Value:    []int{1, 2, 3},
		// 	Encoding: "li1ei2ei3ee",
		// 	Out:      new([]int),
		// },
		// {
		// 	Name:     "heterogenous list",
		// 	Value:    []interface{}{1, "hello"},
		// 	Encoding: "li1e5:helloe",
		// 	Out:      new([]interface{}),
		// },
		// {
		// 	Name:     "list of lists",
		// 	Value:    [][]int{{1, 0}, {0, 1}},
		// 	Encoding: "lli1ei0eeli0ei1eee",
		// 	Out:      new([]int),
		// },
	}

	for _, testCase := range testCases {
		out := testCase.Out
		t.Run(testCase.Name, func(subT *testing.T) {
			s, err := Marshal(testCase.Value)
			if !assert.Nil(subT, err) {
				return
			}

			if !assert.Equal(subT, testCase.Encoding, s) {
				return
			}

			err = Unmarshal(testCase.Encoding, out)
			if !assert.Nil(subT, err) {
				return
			}

			if !assert.Equal(subT, testCase.Value, unref(out)) {
				return
			}
		})
	}
}

func TestUnmarshal(t *testing.T) {
	t.Run("unmarshal type errors", func(subT *testing.T) {
		testCases := []struct {
			Name  string
			Value string
			Out   any
		}{
			{
				Name:  "string",
				Value: "1:a",
				Out:   new(int),
			},
			{
				Name:  "int",
				Value: "i3e",
				Out:   new(string),
			},
		}

		for _, testCase := range testCases {
			out := testCase.Out
			subT.Run(testCase.Name, func(triT *testing.T) {
				err := Unmarshal(testCase.Value, out)
				if !assert.Error(triT, err) {
					return
				}
				if !assert.IsType(triT, &UnmarshalTypeError{}, err) {
					return
				}
			})
		}
	})

	t.Run("invalid encoding", func(subT *testing.T) {
		testCases := []struct {
			Name  string
			Value string
			Out   any
		}{
			{
				Name:  "missing string prefix",
				Value: "hello",
				Out:   new(string),
			},
			{
				Name:  "leading zero",
				Value: "i03e",
				Out:   new(int),
			},
			{
				Name:  "negative zero",
				Value: "i-0e",
				Out:   new(int),
			},
		}

		for _, testCase := range testCases {
			out := testCase.Out
			subT.Run(testCase.Name, func(triT *testing.T) {
				err := Unmarshal(testCase.Value, out)
				if !assert.Error(triT, err) {
					return
				}
				if !assert.IsType(triT, &SyntaxError{}, err) {
					return
				}
			})
		}
	})
}

func unref(v any) any {
	switch x := v.(type) {
	case *string:
		return *x
	case *int:
		return *x
	default:
		return x
	}
}
