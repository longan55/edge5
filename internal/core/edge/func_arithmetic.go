package edge

import (
	"fmt"

	"edge5/internal/core/expr"
)

// arithmetic：自由表达式四则运算（无状态，每个采样周期实时计算）。
// 表达式仅支持数字、输入变量名与 + - * / ( )，例如 (a + b) * c / 2。

type arithmeticFunc struct{}

func (arithmeticFunc) Meta() FuncMeta {
	return FuncMeta{
		Type:    "arithmetic",
		CName:   "四则运算",
		Desc:    "对多个点位做加减乘除，表达式如 (a+b)*c/2，变量名对应输入点位",
		MultiIn: true,
		Params: []ParamMeta{
			{Name: "expression", CName: "表达式", Type: "string", Default: "",
				Desc: "仅支持数字、输入变量与 + - * / ( )，例如 a - b、(a+b)/2"},
		},
	}
}

func (arithmeticFunc) NewState(rule Rule) (State, error) {
	if len(rule.Vars) == 0 {
		return nil, fmt.Errorf("四则运算至少需要 1 个引用变量")
	}
	src := rule.Params.String("expression", "")
	if src == "" {
		return nil, fmt.Errorf("四则运算缺少表达式 expression")
	}
	allowed := make(map[string]bool, len(rule.Vars))
	for _, vr := range rule.Vars {
		allowed[vr.Var] = true
	}
	prog, err := expr.CompileProgram(src, allowed)
	if err != nil {
		return nil, err
	}
	return &arithmeticState{prog: prog}, nil
}

type arithmeticState struct {
	prog *expr.Program
}

func (s *arithmeticState) Evaluate(in map[string]float64, _ string) (interface{}, bool, error) {
	v, err := s.prog.Eval(in)
	if err != nil {
		return nil, false, err
	}
	return v, true, nil
}
