// Package expr 提供安全的四则运算表达式解析与求值。
//
// 仅允许：数字（含小数）、变量名（字母/下划线开头）、+ - * / ( ) 与一元正负号。
// 采用递归下降：expr -> term (('+'|'-') term)*；term -> factor (('*'|'/') factor)*；
// factor -> number | ident | '(' expr ')' | ('+'|'-') factor。
//
// 典型用途：
//   - 采集点位变换：expr 中 x 表示设备读到的原始值，如 x*0.1、(x-4)*2.5+20
//   - 边缘计算四则运算：表达式中使用点位池变量名，如 (a+b)/2
package expr

import (
	"fmt"
	"math"
	"strings"
	"unicode"
)

type tokenKind int

const (
	tkNumber tokenKind = iota
	tkIdent
	tkOp
)

type token struct {
	kind tokenKind
	num  float64
	// frac 数字字面量的小数位数（仅 tkNumber 有效），用于精度控制
	frac int
	text string
}

func tokenize(src string) ([]token, error) {
	toks := make([]token, 0, len(src))
	runes := []rune(strings.TrimSpace(src))
	for i := 0; i < len(runes); {
		c := runes[i]
		switch {
		case unicode.IsSpace(c):
			i++
		case c == '+' || c == '-' || c == '*' || c == '/' || c == '(' || c == ')':
			toks = append(toks, token{kind: tkOp, text: string(c)})
			i++
		case unicode.IsDigit(c) || c == '.':
			j := i
			dot := 0
			for j < len(runes) && (unicode.IsDigit(runes[j]) || runes[j] == '.') {
				if runes[j] == '.' {
					dot++
				}
				j++
			}
			if dot > 1 {
				return nil, fmt.Errorf("非法数字: %s", string(runes[i:j]))
			}
			lit := string(runes[i:j])
			var f float64
			if _, err := fmt.Sscanf(lit, "%g", &f); err != nil {
				return nil, fmt.Errorf("非法数字: %s", lit)
			}
			frac := 0
			if dot == 1 {
				// 小数位数 = 字面量总长 - 小数点位置 - 1
				idx := strings.IndexByte(lit, '.')
				frac = len(lit) - idx - 1
			}
			toks = append(toks, token{kind: tkNumber, num: f, frac: frac})
			i = j
		case c == '_' || unicode.IsLetter(c):
			j := i
			for j < len(runes) && (runes[j] == '_' || unicode.IsLetter(runes[j]) || unicode.IsDigit(runes[j])) {
				j++
			}
			toks = append(toks, token{kind: tkIdent, text: string(runes[i:j])})
			i = j
		default:
			return nil, fmt.Errorf("表达式含非法字符: %c", c)
		}
	}
	return toks, nil
}

// Node 编译后的表达式节点，可重复求值。
type Node interface {
	Eval(vars map[string]float64) (float64, error)
}

type numNode float64

func (n numNode) Eval(map[string]float64) (float64, error) { return float64(n), nil }

type varNode string

func (v varNode) Eval(vars map[string]float64) (float64, error) {
	x, ok := vars[string(v)]
	if !ok {
		return 0, fmt.Errorf("表达式变量未提供: %s", string(v))
	}
	return x, nil
}

type unaryNode struct {
	neg   bool
	child Node
}

func (u unaryNode) Eval(vars map[string]float64) (float64, error) {
	x, err := u.child.Eval(vars)
	if err != nil {
		return 0, err
	}
	if u.neg {
		return -x, nil
	}
	return x, nil
}

type binaryNode struct {
	op   byte
	l, r Node
}

func (b binaryNode) Eval(vars map[string]float64) (float64, error) {
	lv, err := b.l.Eval(vars)
	if err != nil {
		return 0, err
	}
	rv, err := b.r.Eval(vars)
	if err != nil {
		return 0, err
	}
	switch b.op {
	case '+':
		return lv + rv, nil
	case '-':
		return lv - rv, nil
	case '*':
		return lv * rv, nil
	case '/':
		if rv == 0 {
			return 0, fmt.Errorf("表达式除数为零")
		}
		return lv / rv, nil
	}
	return 0, fmt.Errorf("未知运算符: %c", b.op)
}

type parser struct {
	toks []token
	pos  int
}

func (p *parser) peek() *token {
	if p.pos < len(p.toks) {
		return &p.toks[p.pos]
	}
	return nil
}

func (p *parser) next() *token {
	if p.pos < len(p.toks) {
		t := &p.toks[p.pos]
		p.pos++
		return t
	}
	return nil
}

func (p *parser) parseExpr() (Node, error) {
	left, err := p.parseTerm()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t == nil || t.kind != tkOp || (t.text != "+" && t.text != "-") {
			break
		}
		p.next()
		right, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		left = binaryNode{op: t.text[0], l: left, r: right}
	}
	return left, nil
}

func (p *parser) parseTerm() (Node, error) {
	left, err := p.parseFactor()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t == nil || t.kind != tkOp || (t.text != "*" && t.text != "/") {
			break
		}
		p.next()
		right, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		left = binaryNode{op: t.text[0], l: left, r: right}
	}
	return left, nil
}

func (p *parser) parseFactor() (Node, error) {
	t := p.next()
	if t == nil {
		return nil, fmt.Errorf("表达式不完整")
	}
	switch t.kind {
	case tkNumber:
		return numNode(t.num), nil
	case tkIdent:
		return varNode(t.text), nil
	case tkOp:
		if t.text == "(" {
			node, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			cl := p.next()
			if cl == nil || cl.kind != tkOp || cl.text != ")" {
				return nil, fmt.Errorf("缺少右括号")
			}
			return node, nil
		}
		if t.text == "+" || t.text == "-" {
			child, err := p.parseFactor()
			if err != nil {
				return nil, err
			}
			return unaryNode{neg: t.text == "-", child: child}, nil
		}
	}
	return nil, fmt.Errorf("表达式语法错误，位置 %d", p.pos)
}

// Compile 编译表达式，不校验变量集合（变量在 Eval 时缺失会报错）。
func Compile(src string) (Node, error) {
	toks, err := tokenize(src)
	if err != nil {
		return nil, err
	}
	return parse(toks)
}

// CompileAllowed 编译表达式并校验所有变量都在 allowed 集合中。
// 典型用法：点位变换限定只能使用 x；四则运算算子限定只能引用已声明的点位变量。
func CompileAllowed(src string, allowed map[string]bool) (Node, error) {
	toks, err := tokenize(src)
	if err != nil {
		return nil, err
	}
	for _, t := range toks {
		if t.kind == tkIdent && !allowed[t.text] {
			return nil, fmt.Errorf("表达式引用了未定义的变量: %s", t.text)
		}
	}
	return parse(toks)
}

func parse(toks []token) (Node, error) {
	if len(toks) == 0 {
		return nil, fmt.Errorf("表达式为空")
	}
	p := &parser{toks: toks}
	root, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.pos != len(toks) {
		return nil, fmt.Errorf("表达式存在未解析内容，位置 %d", p.pos)
	}
	return root, nil
}

// EvalX 便捷求值：编译一次表达式并用 x 作为原始值变量求值。
// 调用方若需重复求值，应自行 Compile 后缓存 Node。
func EvalX(src string, x float64) (float64, error) {
	root, err := CompileAllowed(src, map[string]bool{"x": true})
	if err != nil {
		return 0, err
	}
	return root.Eval(map[string]float64{"x": x})
}

// Program 编译后的表达式程序，包含 AST 根节点与精度信息。
// Eval 会按表达式中数字字面量的最大小数位数四舍五入，消除浮点误差：
//   - x*0.01    → 最大小数位数 2，结果按 2 位小数四舍五入
//   - x*0.001   → 最大小数位数 3，结果按 3 位小数四舍五入
//   - x/3       → 无小数字面量，不四舍五入（保留原值）
//
// 精度仅取自数字字面量，变量值的小数位数不影响（因为变量是运行时输入）。
type Program struct {
	root      Node
	precision int // 表达式中数字字面量的最大小数位数；0 表示不四舍五入
}

// CompileProgram 编译表达式并提取精度信息，返回可重复求值的 *Program。
// allowed 为允许的变量名集合，引用未在集合中的变量会报错。
func CompileProgram(src string, allowed map[string]bool) (*Program, error) {
	toks, err := tokenize(src)
	if err != nil {
		return nil, err
	}
	for _, t := range toks {
		if t.kind == tkIdent && !allowed[t.text] {
			return nil, fmt.Errorf("表达式引用了未定义的变量: %s", t.text)
		}
	}
	root, err := parse(toks)
	if err != nil {
		return nil, err
	}
	return &Program{root: root, precision: maxFrac(toks)}, nil
}

// Eval 求值并按精度四舍五入（precision>0 时）。
func (p *Program) Eval(vars map[string]float64) (float64, error) {
	v, err := p.root.Eval(vars)
	if err != nil {
		return 0, err
	}
	if p.precision > 0 {
		return round(v, p.precision), nil
	}
	return v, nil
}

// Precision 返回表达式中数字字面量的最大小数位数（0 表示无小数字面量）。
func (p *Program) Precision() int { return p.precision }

// maxFrac 统计 token 列表中数字字面量的最大小数位数
func maxFrac(toks []token) int {
	m := 0
	for _, t := range toks {
		if t.kind == tkNumber && t.frac > m {
			m = t.frac
		}
	}
	return m
}

// round 按 precision 位小数四舍五入（Banker's rounding 不适用，用 math.Round 半远离零）
func round(v float64, precision int) float64 {
	pow := 1.0
	for i := 0; i < precision; i++ {
		pow *= 10
	}
	return math.Round(v*pow) / pow
}
