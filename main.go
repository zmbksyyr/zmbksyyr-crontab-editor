package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time" // 导入 time 包以处理时间相关逻辑
)

const (
	port = ":8080"
)

// CrontabEntry 结构体表示一个 crontab 任务
type CrontabEntry struct {
	ID      int    `json:"id"`
	Minute  string `json:"minute"`
	Hour    string `json:"hour"`
	DayOfMonth string `json:"dayOfMonth"`
	Month   string `json:"month"`
	DayOfWeek string `json:"dayOfWeek"`
	Command string `json:"command"`
	RawLine string `json:"rawLine"` // 存储原始行，用于修改
	Comment string `json:"comment"` // 如果有注释，也一并存储
	Enabled bool   `json:"enabled"` // 是否启用
}

// Global variable to keep track of the last known crontab entries
var currentCrontabEntries []CrontabEntry
var nextEntryID int // Used to assign unique IDs to entries

func main() {
	// 初始化 nextEntryID
	nextEntryID = 1

	// 注册路由
	http.HandleFunc("/", serveIndex)
	http.HandleFunc("/api/crontab", handleCrontabAPI)

	// 提供静态文件
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	log.Printf("Server listening on http://localhost%s", port)
	log.Fatal(http.ListenAndServe(port, nil))
}

// serveIndex 渲染 HTML 模板
func serveIndex(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, nil)
}

// handleCrontabAPI 处理获取和更新 crontab 任务的 API 请求
func handleCrontabAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		getCrontab(w, r)
	case "POST":
		updateCrontab(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// getCrontab 获取当前用户的 crontab 任务
func getCrontab(w http.ResponseWriter, r *http.Request) {
	cmd := exec.Command("crontab", "-l")
	output, err := cmd.Output()
	if err != nil {
		// 如果 crontab 为空，会返回错误，stderr 可能是 "no crontab for user"
		if exitErr, ok := err.(*exec.ExitError); ok {
			if strings.Contains(string(exitErr.Stderr), "no crontab for") {
				// 认为是空的 crontab，返回空列表
				json.NewEncoder(w).Encode([]CrontabEntry{})
				return
			}
		}
		log.Printf("Error executing crontab -l: %v, Output: %s", err, string(output))
		http.Error(w, fmt.Sprintf("Failed to list crontab: %v", err), http.StatusInternalServerError)
		return
	}

	entries := parseCrontabOutput(string(output))
	currentCrontabEntries = entries // 更新全局变量
	json.NewEncoder(w).Encode(entries)
}

// parseCrontabOutput 解析 crontab -l 的输出
func parseCrontabOutput(output string) []CrontabEntry {
	var entries []CrontabEntry
	scanner := bufio.NewScanner(strings.NewReader(output))
	cronRegex := regexp.MustCompile(`^(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(.*)$`) // 匹配时间字段和命令

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		// 忽略环境变量和非任务行
		if strings.HasPrefix(line, "#") {
			// 处理被注释掉的任务
			if strings.HasPrefix(line, "# ") { // 可能是我们自己注释掉的任务
				// 尝试解析被注释掉的任务行
				if matches := cronRegex.FindStringSubmatch(line[2:]); len(matches) == 7 { // 跳过 "# "
					entries = append(entries, CrontabEntry{
						ID:      nextEntryID,
						Minute:  matches[1],
						Hour:    matches[2],
						DayOfMonth: matches[3],
						Month:   matches[4],
						DayOfWeek: matches[5],
						Command: strings.TrimSpace(matches[6]),
						RawLine: line,
						Comment: "", // No separate comment for disabled entries for simplicity
						Enabled: false,
					})
					nextEntryID++
					continue
				}
			}
			// 其他类型的注释，直接跳过
			continue
		}
		if strings.Contains(line, "=") && !strings.HasPrefix(line, "*") { // 可能是环境变量
			continue
		}

		if matches := cronRegex.FindStringSubmatch(line); len(matches) == 7 {
			entries = append(entries, CrontabEntry{
				ID:      nextEntryID,
				Minute:  matches[1],
				Hour:    matches[2],
				DayOfMonth: matches[3],
				Month:   matches[4],
				DayOfWeek: matches[5],
				Command: strings.TrimSpace(matches[6]),
				RawLine: line,
				Comment: "",
				Enabled: true,
			})
			nextEntryID++
		} else {
			// 如果不符合标准的 cron 格式，但也不是注释，就作为原始行保存
			log.Printf("Warning: Non-standard crontab line skipped: %s", line)
			// 或者你可以选择将其作为一个特殊条目来处理
		}
	}
	return entries
}

// updateCrontab 更新 crontab 任务
func updateCrontab(w http.ResponseWriter, r *http.Request) {
	var updatedEntries []CrontabEntry
	if err := json.NewDecoder(r.Body).Decode(&updatedEntries); err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse request body: %v", err), http.StatusBadRequest)
		return
	}

	// 重新构建完整的 crontab 内容
	var newCrontabContent bytes.Buffer
	existingRawLines := make(map[string]bool) // 记录已处理的原始行，避免重复

	// 优先添加所有 enabled 的新行
	for _, entry := range updatedEntries {
		if entry.Enabled {
			newLine := fmt.Sprintf("%s %s %s %s %s %s",
				entry.Minute, entry.Hour, entry.DayOfMonth, entry.Month, entry.DayOfWeek, entry.Command)
			newCrontabContent.WriteString(newLine)
			newCrontabContent.WriteString("\n")
			existingRawLines[entry.RawLine] = true // 标记原始行已处理
		} else {
			// 对于 disabled 的条目，如果它之前是 enabled 状态，则把它注释掉
			// 如果它已经是注释状态，则保持注释
			if entry.RawLine != "" && !strings.HasPrefix(entry.RawLine, "#") {
				// 说明之前是 enabled 的，现在被 disable 了
				newCrontabContent.WriteString(fmt.Sprintf("# %s\n", entry.RawLine))
				existingRawLines[entry.RawLine] = true
			} else if entry.RawLine != "" && strings.HasPrefix(entry.RawLine, "#") {
				// 之前就是 disabled 的
				newCrontabContent.WriteString(fmt.Sprintf("%s\n", entry.RawLine))
				existingRawLines[entry.RawLine] = true
			} else {
				// 新增的 disabled 任务，直接以注释形式添加
				newLine := fmt.Sprintf("# %s %s %s %s %s %s",
					entry.Minute, entry.Hour, entry.DayOfMonth, entry.Month, entry.DayOfWeek, entry.Command)
				newCrontabContent.WriteString(newLine)
				newCrontabContent.WriteString("\n")
				existingRawLines[entry.RawLine] = true // 虽然是新的，但仍然标记一下
			}
		}
	}

	// 读取当前的 crontab 以保留非任务行 (如环境变量，其他注释)
	cmd := exec.Command("crontab", "-l")
	output, err := cmd.Output()
	if err != nil {
		// 如果 crontab 为空，则不需要合并其他行
		if exitErr, ok := err.(*exec.ExitError); ok && strings.Contains(string(exitErr.Stderr), "no crontab for") {
			// 继续执行，因为没有要合并的旧行
		} else {
			log.Printf("Error executing crontab -l for merge: %v, Output: %s", err, string(output))
			http.Error(w, fmt.Sprintf("Failed to list crontab for merge: %v", err), http.StatusInternalServerError)
			return
		}
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	cronRegex := regexp.MustCompile(`^(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(.*)$`) // 用于判断是否为 crontab 任务行

	for scanner.Scan() {
		line := scanner.Text()
		trimmedLine := strings.TrimSpace(line)

		// 如果是空行或注释，且不是我们管理的任务行，则保留
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "#") {
			// 但要确保它不是我们通过 disable 机制生成的注释行
			// 否则如果用户在编辑器里把一个注释行修改成了普通行，可能导致重复
			// 这里简单地检查一下原始行是否在我们的更新列表中，以避免重复添加
			isManagedComment := false
			for _, entry := range updatedEntries {
				// Check if the current line is the rawLine of any updated entry
				// This comparison might need to be more robust for real-world scenarios
				if entry.RawLine == line {
					isManagedComment = true
					break
				}
				// Also check if it's a commented version of an updated entry's command
				if !entry.Enabled && strings.HasPrefix(line, "# ") {
					testLine := line[2:]
					if cronRegex.MatchString(testLine) {
						// This is a bit tricky, needs careful logic
						// For now, assume if it matches an updated entry's rawLine, it's handled.
					}
				}
			}

			if !isManagedComment && !cronRegex.MatchString(line) && !strings.HasPrefix(line, "# ") {
				// 如果不是标准的 cron 任务行，也不是我们生成的注释行，就保留
				// 还需要检查它是否已经被包含在 updatedEntries 中
				found := false
				for _, entry := range updatedEntries {
					if entry.RawLine == line {
						found = true
						break
					}
				}
				if !found {
					newCrontabContent.WriteString(line)
					newCrontabContent.WriteString("\n")
				}
			}
			continue
		}

		// 如果是旧的 cron 任务行，检查它是否已在 updatedEntries 中被处理
		foundInUpdated := false
		for _, entry := range updatedEntries {
			// 精确匹配原始行
			if entry.RawLine == line {
				foundInUpdated = true
				break
			}
			// 也要考虑原始行是注释，但现在变成启用状态的情况
			if strings.HasPrefix(line, "# ") {
				if entry.Enabled && fmt.Sprintf("%s %s %s %s %s %s", entry.Minute, entry.Hour, entry.DayOfMonth, entry.Month, entry.DayOfWeek, entry.Command) == line[2:] {
					foundInUpdated = true
					break
				}
			}
		}

		// 如果是一个旧的非任务行（如环境变量），并且没有被更新的条目取代，则保留
		if !cronRegex.MatchString(line) && !foundInUpdated {
			newCrontabContent.WriteString(line)
			newCrontabContent.WriteString("\n")
		}
	}

	// 写入临时文件
	tmpfile, err := os.CreateTemp("", "crontab-editor-")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create temp file: %v", err), http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmpfile.Name()) // 确保临时文件被删除

	if _, err := tmpfile.WriteString(newCrontabContent.String()); err != nil {
		http.Error(w, fmt.Sprintf("Failed to write to temp file: %v", err), http.StatusInternalServerError)
		return
	}
	tmpfile.Close()

	// 使用 crontab 命令更新
	cmd = exec.Command("crontab", tmpfile.Name())
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Printf("Error updating crontab: %v, Output: %s, Crontab Content:\n%s", err, string(output), newCrontabContent.String())
		http.Error(w, fmt.Sprintf("Failed to update crontab: %v, Output: %s", err, string(output)), http.StatusInternalServerError)
		return
	}

	// 成功后重新获取并返回最新的 crontab
	getCrontab(w, r) // 调用 GET 方法来获取最新的 crontab，并返回给前端
}
