package cli

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"golang.org/x/term"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 通用工具函数
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ParseIntWithDefault 解析整数，失败返回默认值
func ParseIntWithDefault(s string, defaultVal int) (int, error) {
	if s == "" {
		return defaultVal, nil
	}

	var val int
	_, err := fmt.Sscanf(s, "%d", &val)
	if err != nil {
		return 0, fmt.Errorf("invalid number: %s", s)
	}
	return val, nil
}

// Truncate 截断字符串到指定长度（导出）
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 2 {
		return s[:maxLen]
	}
	return s[:maxLen-2] + ".."
}

// FormatTime 格式化时间字符串
func FormatTime(timeStr string) string {
	if timeStr == "" {
		return "N/A"
	}

	// 尝试解析ISO 8601格式
	t, err := time.Parse(time.RFC3339, timeStr)
	if err == nil {
		return t.Format("2006-01-02 15:04")
	}

	// 简化时间显示（去掉秒和时区）
	if len(timeStr) > 16 {
		return timeStr[:16]
	}
	return timeStr
}

// FormatBytes 格式化字节数
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// FormatDuration 格式化时长
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())/24)
}

// PromptSelect 交互式选择（支持光标上下移动）
// 返回选中的索引，如果取消则返回 -1
func PromptSelect(prompt string, options []string) (int, error) {
	if len(options) == 0 {
		return -1, fmt.Errorf("no options provided")
	}

	// 保存终端状态
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return -1, fmt.Errorf("failed to enter raw mode: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	selected := 0
	reader := bufio.NewReader(os.Stdin)
	totalLines := len(options) + 1 // 提示行 + 选项行

	// 清除选择界面的辅助函数
	clearSelect := func() {
		fmt.Print("\033[?25h") // 显示光标
		// 清除所有行（选项行 + 提示行）
		// 从最后一行开始清除，确保正确清除
		for i := 0; i < totalLines; i++ {
			fmt.Print("\033[1A") // 上移一行
			fmt.Print("\033[2K") // 清除整行
			fmt.Print("\r")      // 回到行首
		}
		os.Stdout.Sync()
	}

	// 显示选择界面
	renderSelect := func() {
		fmt.Print("\033[?25l") // 隐藏光标
		// 清除之前的内容：上移并清除所有行
		for i := 0; i < totalLines; i++ {
			fmt.Print("\033[1A") // 上移一行
			fmt.Print("\033[2K") // 清除整行
			fmt.Print("\r")      // 回到行首
		}

		// 重新绘制提示行（单独一行，确保换行）
		fmt.Fprintf(os.Stdout, "\r%s\n", prompt)

		// 重新绘制所有选项（每行独立，确保对齐，每行都从行首开始）
		for i, opt := range options {
			if i == selected {
				fmt.Fprintf(os.Stdout, "\r\033[1;32m> %s\033[0m\n", opt) // 绿色高亮选中项
			} else {
				fmt.Fprintf(os.Stdout, "\r  %s\n", opt) // 两个空格对齐
			}
		}
		fmt.Print("\033[?25h") // 显示光标
		os.Stdout.Sync()
	}

	// 初始显示（提示行单独一行，选项从新行开始，确保对齐）
	// 每行都从行首开始，确保对齐
	fmt.Fprintf(os.Stdout, "\r%s\n", prompt)
	for i, opt := range options {
		if i == selected {
			fmt.Fprintf(os.Stdout, "\r\033[1;32m> %s\033[0m\n", opt) // 绿色高亮选中项
		} else {
			fmt.Fprintf(os.Stdout, "\r  %s\n", opt) // 两个空格对齐
		}
	}
	os.Stdout.Sync()

	for {
		char, _, err := reader.ReadRune()
		if err != nil {
			clearSelect()
			return -1, err
		}

		switch char {
		case '\x1b': // ESC 序列开始
			// 读取后续字符判断是否是方向键
			next1, _, err := reader.ReadRune()
			if err != nil {
				clearSelect()
				return -1, err
			}
			if next1 == '[' {
				next2, _, err := reader.ReadRune()
				if err != nil {
					clearSelect()
					return -1, err
				}
				switch next2 {
				case 'A': // 上箭头
					if selected > 0 {
						selected--
						renderSelect()
					}
				case 'B': // 下箭头
					if selected < len(options)-1 {
						selected++
						renderSelect()
					}
				}
			} else {
				// ESC 键，取消
				clearSelect()
				return -1, nil
			}
		case '\r', '\n': // Enter 确认
			clearSelect()
			fmt.Printf("%s%s\n", prompt, options[selected])
			return selected, nil
		case '\x03': // Ctrl+C - 静默返回，不显示错误
			clearSelect()
			return -1, nil
		}
	}
}
