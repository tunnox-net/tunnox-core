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
	fmt.Printf("%s\n", colorError(msg))
}

// Warning 输出警告消息
func (o *Output) Warning(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s\n", colorWarning(msg))
}

// Info 输出信息消息
func (o *Output) Info(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s\n", colorInfo(msg))
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
	// 计算每列的最大宽度（考虑颜色代码）
	widths := make([]int, len(t.headers))
	for i, header := range t.headers {
		// 计算实际显示宽度：对表头使用 colorBold 后的实际宽度
		boldHeader := colorBold(header)
		widths[i] = len(stripANSI(boldHeader))
	}
	
	// 更新宽度，考虑数据行的内容
	for _, row := range t.rows {
		for i := 0; i < len(t.headers) && i < len(row); i++ {
			actualWidth := len(stripANSI(row[i]))
			if actualWidth > widths[i] {
				widths[i] = actualWidth
			}
		}
	}
	
	// 确保最小宽度
	for i := range widths {
		if widths[i] < 3 {
			widths[i] = 3
		}
	}

	// 打印表头
	for i, header := range t.headers {
		if i > 0 {
			fmt.Print("  ")
		}
		boldHeader := colorBold(header)
		// 计算实际显示宽度
		displayWidth := len(stripANSI(boldHeader))
		// 打印带颜色的表头，然后手动添加填充
		fmt.Print(boldHeader)
		if displayWidth < widths[i] {
			// 需要添加填充：目标宽度 - 实际显示宽度
			fmt.Print(strings.Repeat(" ", widths[i]-displayWidth))
		}
	}
	fmt.Println()

	// 打印分隔线
	totalWidth := 0
	for _, w := range widths {
		totalWidth += w + 2
	}
	if totalWidth > 2 {
		totalWidth -= 2 // 最后一列不需要后面的空格
	}
	fmt.Println(strings.Repeat("─", min(totalWidth, 120)))

	// 打印数据行
	for _, row := range t.rows {
		for i := 0; i < len(t.headers); i++ {
			if i > 0 {
				fmt.Print("  ")
			}
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			// 对于数据行，也需要考虑可能的颜色代码
			displayWidth := len(stripANSI(cell))
			fmt.Print(cell)
			if displayWidth < widths[i] {
				fmt.Print(strings.Repeat(" ", widths[i]-displayWidth))
			}
		}
		fmt.Println()
	}
}

// stripANSI 移除ANSI颜色代码，计算实际显示宽度
func stripANSI(s string) string {
	var result strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
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
