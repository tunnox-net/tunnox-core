package cli

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 彩色输出工具 (P1.4)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

var (
	// 颜色函数
	colorSuccess = color.New(color.FgGreen).SprintFunc()
	colorError   = color.New(color.FgRed).SprintFunc()
	colorWarning = color.New(color.FgYellow).SprintFunc()
	colorInfo    = color.New(color.FgCyan).SprintFunc()
	colorBold    = color.New(color.Bold).SprintFunc()
	colorFaint   = color.New(color.Faint).SprintFunc()
)

// Output 提供结构化的输出接口
type Output struct {
	noColor bool
}

// NewOutput 创建输出工具
func NewOutput(noColor bool) *Output {
	color.NoColor = noColor
	return &Output{noColor: noColor}
}

// Success 输出成功消息
func (o *Output) Success(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s %s\n", colorSuccess("✅"), msg)
}

// Error 输出错误消息
func (o *Output) Error(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s %s\n", colorError("❌"), msg)
}

// Warning 输出警告消息
func (o *Output) Warning(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s %s\n", colorWarning("⚠️"), msg)
}

// Info 输出信息消息
func (o *Output) Info(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s %s\n", colorInfo("ℹ️"), msg)
}

// Plain 输出普通消息（无颜色）
func (o *Output) Plain(format string, args ...interface{}) {
	fmt.Printf(format+"\n", args...)
}

// Header 输出标题
func (o *Output) Header(title string) {
	fmt.Println("")
	fmt.Println(colorBold(title))
	fmt.Println(strings.Repeat("━", len(title)))
	fmt.Println("")
}

// Section 输出分节标题
func (o *Output) Section(title string) {
	fmt.Println("")
	fmt.Println(colorBold(title))
	fmt.Println(strings.Repeat("─", min(len(title), 80)))
	fmt.Println("")
}

// Table 输出表格
type Table struct {
	headers []string
	rows    [][]string
	widths  []int
}

// NewTable 创建新表格
func NewTable(headers ...string) *Table {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	return &Table{
		headers: headers,
		rows:    make([][]string, 0),
		widths:  widths,
	}
}

// AddRow 添加行
func (t *Table) AddRow(cols ...string) {
	// 更新列宽
	for i, col := range cols {
		if i < len(t.widths) && len(col) > t.widths[i] {
			t.widths[i] = len(col)
		}
	}
	t.rows = append(t.rows, cols)
}

// Render 渲染表格
func (t *Table) Render() {
	// 打印表头
	for i, header := range t.headers {
		fmt.Printf("%-*s  ", t.widths[i], colorBold(header))
	}
	fmt.Println()

	// 打印分隔线
	totalWidth := 0
	for _, w := range t.widths {
		totalWidth += w + 2
	}
	fmt.Println(strings.Repeat("─", min(totalWidth, 120)))

	// 打印数据行
	for _, row := range t.rows {
		for i, col := range row {
			if i < len(t.widths) {
				fmt.Printf("%-*s  ", t.widths[i], col)
			}
		}
		fmt.Println()
	}
}

// KeyValue 输出键值对
func (o *Output) KeyValue(key, value string) {
	fmt.Printf("  %-20s %s\n", colorBold(key+":"), value)
}

// Separator 输出分隔线
func (o *Output) Separator() {
	fmt.Println(colorFaint(strings.Repeat("━", 80)))
}

// min 返回两个整数的最小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
