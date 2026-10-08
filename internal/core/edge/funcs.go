package edge

// 内置算子在此注册。新增算子：新建文件实现 ComputeFunc，并在下方 init 追加 Register。
func init() {
	Register(digitalCounterFunc{})
	Register(analogCounterFunc{})
	Register(arithmeticFunc{})
}
