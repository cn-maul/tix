package handler

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/signintech/gopdf"
	"github.com/vicanso/go-charts/v2"
	"tix/internal/database"
	"tix/internal/model"
)

func (h *Handler) ExportReport(w http.ResponseWriter, r *http.Request) {
	startDate := r.URL.Query().Get("start_date")
	endDate := r.URL.Query().Get("end_date")

	opts := database.ListOptions{Limit: 10000, Offset: 0}
	if startDate != "" {
		opts.StartDate = startDate
	}
	if endDate != "" {
		opts.EndDate = endDate
	}

	resp, err := h.svc.List(opts)
	if err != nil {
		h.error(w, 500, "LIST_ERROR", "获取工单失败")
		return
	}

	if len(resp.Items) == 0 {
		h.error(w, 400, "NO_TICKETS", "没有工单可导出")
		return
	}

	// 获取字体路径
	fontPath := h.cfg.PDF.FontPath
	if fontPath == "" {
		h.error(w, 500, "NO_FONT", "未配置PDF字体路径，请在设置中配置")
		return
	}

	pdf, err := generateReport(resp.Items, startDate, endDate, fontPath)
	if err != nil {
		log.Printf("生成报告失败: %v", err)
		h.error(w, 500, "PDF_ERROR", "生成PDF失败")
		return
	}

	w.Header().Set("Content-Type", "application/pdf")

	// 生成文件名
	now := time.Now()
	filename := fmt.Sprintf("%d年%d月工作报告.pdf", now.Year(), int(now.Month()))
	if startDate != "" && len(startDate) >= 7 {
		filename = fmt.Sprintf("%s月工作报告.pdf", startDate[:7])
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Write(pdf)
}

func generateReport(tickets []model.Ticket, startDate, endDate, fontPath string) ([]byte, error) {
	pdf := gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})

	if err := pdf.AddTTFFont("wqy", fontPath); err != nil {
		return nil, fmt.Errorf("添加字体失败: %v", err)
	}
	if err := pdf.SetFont("wqy", "", 14); err != nil {
		return nil, fmt.Errorf("设置字体失败: %v", err)
	}

	stats := calculateStats(tickets)

	// ========== 第1页：标题、统计、图表 ==========
	pdf.AddPage()

	// 标题：根据时间范围生成
	pdf.SetFont("wqy", "", 28)
	pdf.SetY(40)

	// 确定报告年月
	reportTitle := ""
	now := time.Now()
	if startDate != "" && len(startDate) >= 7 {
		// 从筛选开始日期提取年月
		reportTitle = fmt.Sprintf("%s月工作报告", startDate[:7])
	} else {
		// 默认当前年月
		reportTitle = fmt.Sprintf("%d年%d月工作报告", now.Year(), int(now.Month()))
	}
	pdf.CellWithOption(&gopdf.Rect{W: 595, H: 40}, reportTitle, gopdf.CellOption{Align: gopdf.Center})

	// 时间范围
	pdf.SetFont("wqy", "", 14)
	pdf.SetY(85)
	timeRange := "全部时间"
	if startDate != "" && endDate != "" {
		timeRange = fmt.Sprintf("%s 至 %s", startDate, endDate)
	} else if startDate != "" {
		timeRange = fmt.Sprintf("%s 起", startDate)
	} else if endDate != "" {
		timeRange = fmt.Sprintf("至 %s", endDate)
	}
	pdf.CellWithOption(&gopdf.Rect{W: 595, H: 20}, timeRange, gopdf.CellOption{Align: gopdf.Center})

	// 生成时间
	pdf.SetY(105)
	pdf.CellWithOption(&gopdf.Rect{W: 595, H: 20}, fmt.Sprintf("生成时间: %s", time.Now().Format("2006-01-02 15:04")), gopdf.CellOption{Align: gopdf.Center})

	// 分隔线
	pdf.SetLineWidth(1)
	pdf.Line(50, 135, 545, 135)

	// ===== 统计概览 =====
	pdf.SetFont("wqy", "", 20)
	pdf.SetY(150)
	pdf.SetX(50)
	pdf.Cell(nil, "统计概览")

	// 三个统计卡片
	cardY := 180.0
	cardWidth := 150.0
	cardHeight := 70.0
	cardSpacing := 15.0

	drawStatCard(&pdf, 50, cardY, cardWidth, cardHeight, "工单总数", fmt.Sprintf("%d", stats.Total), "32,128,208")
	drawStatCard(&pdf, 50+cardWidth+cardSpacing, cardY, cardWidth, cardHeight, "已完成", fmt.Sprintf("%d", stats.Completed), "34,197,94")
	drawStatCard(&pdf, 50+2*(cardWidth+cardSpacing), cardY, cardWidth, cardHeight, "处理中", fmt.Sprintf("%d", stats.Pending), "239,68,68")

	// 完成率
	percent := 0.0
	if stats.Total > 0 {
		percent = float64(stats.Completed) / float64(stats.Total) * 100
	}
	pdf.SetFont("wqy", "", 14)
	pdf.SetY(cardY + cardHeight + 15)
	pdf.SetX(50)
	pdf.Cell(nil, fmt.Sprintf("完成率: %.1f%%", percent))

	// ===== 分类统计 =====
	pdf.SetFont("wqy", "", 20)
	pdf.SetY(290)
	pdf.SetX(50)
	pdf.Cell(nil, "分类统计")

	// 排序
	type catCount struct {
		cat   string
		count int
	}
	var categories []catCount
	for cat, count := range stats.ByCategory {
		categories = append(categories, catCount{cat, count})
	}
	sort.Slice(categories, func(i, j int) bool {
		return categories[i].count > categories[j].count
	})

	// 分类列表（带进度条）
	pdf.SetFont("wqy", "", 12)
	y := 320.0
	for _, cc := range categories {
		pdf.SetY(y)
		pdf.SetX(50)
		pdf.Cell(nil, cc.cat)

		// 进度条背景
		barWidth := 200.0
		barX := 150.0
		pdf.SetFillColor(230, 230, 230)
		pdf.Rectangle(barX, y+2, barX+barWidth, y+14, "D", 0, 0)

		// 进度条填充
		fillWidth := barWidth * float64(cc.count) / float64(stats.Total)
		pdf.SetFillColor(59, 130, 246)
		pdf.Rectangle(barX, y+2, barX+fillWidth, y+14, "F", 0, 0)

		// 数量和百分比
		pdf.SetX(barX + barWidth + 10)
		pdf.Cell(nil, fmt.Sprintf("%d (%.1f%%)", cc.count, float64(cc.count)/float64(stats.Total)*100))

		y += 25
	}

	// ===== 图表（放到第一页下半部分）=====
	// 加载字体给图表
	fontData, err := os.ReadFile(fontPath)
	if err == nil {
		charts.InstallFont("wqy", fontData)
		if f, _ := charts.GetFont("wqy"); f != nil {
			charts.SetDefaultFont(f)
		}
	}

	// 图表位置
	chartY := 450.0

	// 饼图标题
	pdf.SetFont("wqy", "", 16)
	pdf.SetY(chartY)
	pdf.SetX(50)
	pdf.Cell(nil, "分类分布")

	// 饼图
	pieBuf, err := createPieChart(stats.ByCategory, stats.Total)
	if err == nil {
		imgHolder, _ := gopdf.ImageHolderByBytes(pieBuf)
		if imgHolder != nil {
			pdf.ImageByHolder(imgHolder, 50, chartY+20, &gopdf.Rect{W: 220, H: 180})
		}
	}

	// 柱状图标题
	pdf.SetY(chartY)
	pdf.SetX(330)
	pdf.Cell(nil, "每日工单趋势")

	// 柱状图
	barBuf, err := createBarChart(stats.ByDate)
	if err == nil {
		imgHolder, _ := gopdf.ImageHolderByBytes(barBuf)
		if imgHolder != nil {
			pdf.ImageByHolder(imgHolder, 330, chartY+20, &gopdf.Rect{W: 220, H: 180})
		}
	}

	// ========== 第2页起：工单明细 ==========
	pdf.AddPage()

	pdf.SetFont("wqy", "", 20)
	pdf.SetY(30)
	pdf.SetX(50)
	pdf.Cell(nil, "工单明细")

	y = 65.0

	for i, t := range tickets {
		// 检查是否需要新页（提前判断）
		cardHeight := 90.0
		if t.Content != "" && t.Title != "" {
			cardHeight += 18
		}
		if t.Resolution != "" {
			// 处理过程可能很长，预估行数
			resolutionLines := (len([]rune(t.Resolution)) / 50) + 1
			cardHeight += float64(resolutionLines) * 14
		}

		if y+cardHeight > 780 {
			pdf.AddPage()
			y = 40
		}

		// 卡片背景
		cardTop := y - 3
		pdf.SetFillColor(248, 250, 252)
		pdf.Rectangle(35, cardTop, 560, cardTop+cardHeight, "F", 0, 0)

		// 序号
		pdf.SetFont("wqy", "", 13)
		pdf.SetY(y)
		pdf.SetX(45)
		pdf.Cell(nil, fmt.Sprintf("#%d", i+1))

		// 状态标签
		if t.IsCompleted {
			pdf.SetFillColor(34, 197, 94)
		} else {
			pdf.SetFillColor(239, 68, 68)
		}
		statusText := "已完成"
		if !t.IsCompleted {
			statusText = "处理中"
		}
		pdf.Rectangle(500, y, 550, y+18, "F", 0, 0)
		pdf.SetFont("wqy", "", 9)
		pdf.SetTextColor(255, 255, 255)
		pdf.SetY(y + 3)
		pdf.SetX(508)
		pdf.Cell(nil, statusText)
		pdf.SetTextColor(0, 0, 0)

		// 第一行：发起人 | 分类 | 创建时间
		pdf.SetFont("wqy", "", 10)
		y += 22
		pdf.SetY(y)
		pdf.SetX(45)
		pdf.Cell(nil, fmt.Sprintf("发起人: %s", truncate(t.Initiator, 8)))
		pdf.SetX(160)
		pdf.Cell(nil, fmt.Sprintf("分类: %s", truncate(t.Category, 10)))
		pdf.SetX(280)
		if len(t.CreatedAt) >= 10 {
			pdf.Cell(nil, fmt.Sprintf("创建: %s", t.CreatedAt[:10]))
		}

		// 标题
		y += 18
		pdf.SetY(y)
		pdf.SetX(45)
		title := t.Title
		if title == "" {
			title = truncate(t.Content, 45)
		}
		pdf.SetFont("wqy", "", 11)
		pdf.Cell(nil, fmt.Sprintf("标题: %s", truncate(title, 45)))

		// 内容（如果有）
		if t.Content != "" && t.Title != "" {
			y += 16
			pdf.SetY(y)
			pdf.SetX(45)
			pdf.SetFont("wqy", "", 9)
			pdf.SetTextColor(80, 80, 80)
			y = drawWrappedText(&pdf, "内容: ", t.Content, 45, 500, y)
			pdf.SetTextColor(0, 0, 0)
		}

		// 处理过程（如果有）
		if t.Resolution != "" {
			y += 16
			pdf.SetY(y)
			pdf.SetX(45)
			pdf.SetFont("wqy", "", 9)
			pdf.SetTextColor(22, 163, 74)
			y = drawWrappedText(&pdf, "处理: ", t.Resolution, 45, 500, y)
			pdf.SetTextColor(0, 0, 0)
		}

		y += 20
	}

	var buf bytes.Buffer
	_, err = pdf.WriteTo(&buf)
	return buf.Bytes(), err
}

// 绘制多行文本，返回结束Y坐标
func drawWrappedText(pdf *gopdf.GoPdf, prefix, text string, x, maxWidth, startY float64) float64 {
	labelWidth := 35.0
	pdf.SetX(x)
	pdf.Cell(nil, prefix)

	// 每行约50个中文字符
	maxChars := 50
	text = strings.TrimSpace(text)
	runes := []rune(text)

	if len(runes) <= maxChars {
		pdf.SetX(x + labelWidth)
		pdf.Cell(nil, text)
		return startY
	}

	// 需要换行
	firstLine := string(runes[:maxChars])
	pdf.SetX(x + labelWidth)
	pdf.Cell(nil, firstLine)

	remaining := runes[maxChars:]
	y := startY

	for len(remaining) > 0 {
		y += 13
		pdf.SetY(y)
		pdf.SetX(x + labelWidth)

		lineLen := min(maxChars, len(remaining))
		pdf.Cell(nil, string(remaining[:lineLen]))
		remaining = remaining[lineLen:]
	}

	return y
}

func drawStatCard(pdf *gopdf.GoPdf, x, y, w, h float64, label, value, color string) {
	var r, g, b int
	fmt.Sscanf(color, "%d,%d,%d", &r, &g, &b)

	pdf.SetFillColor(uint8(r), uint8(g), uint8(b))
	pdf.Rectangle(x, y, x+w, y+h, "F", 0, 0)

	pdf.SetFont("wqy", "", 11)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetY(y + 15)
	pdf.SetX(x + 15)
	pdf.Cell(nil, label)

	pdf.SetFont("wqy", "", 28)
	pdf.SetY(y + 32)
	pdf.SetX(x + 15)
	pdf.Cell(nil, value)

	pdf.SetTextColor(0, 0, 0)
}

type Stats struct {
	Total      int
	Completed  int
	Pending    int
	ByCategory map[string]int
	ByDate     map[string]int
}

func calculateStats(tickets []model.Ticket) Stats {
	stats := Stats{
		ByCategory: make(map[string]int),
		ByDate:     make(map[string]int),
	}

	for _, t := range tickets {
		stats.Total++
		if t.IsCompleted {
			stats.Completed++
		} else {
			stats.Pending++
		}
		if t.Category != "" {
			stats.ByCategory[t.Category]++
		}
		if len(t.CreatedAt) >= 10 {
			date := t.CreatedAt[:10]
			stats.ByDate[date]++
		}
	}

	return stats
}

func createPieChart(data map[string]int, total int) ([]byte, error) {
	type kv struct {
		key string
		val int
	}
	var sorted []kv
	for k, v := range data {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].val > sorted[j].val
	})
	if len(sorted) > 6 {
		sorted = sorted[:6]
	}

	var values []float64
	var labels []string
	for _, kv := range sorted {
		values = append(values, float64(kv.val))
		labels = append(labels, fmt.Sprintf("%s (%.0f%%)", kv.key, float64(kv.val)/float64(total)*100))
	}

	p, err := charts.PieRender(values,
		charts.TitleOptionFunc(charts.TitleOption{Text: ""}),
		charts.PieSeriesShowLabel(),
		charts.WidthOptionFunc(240),
		charts.HeightOptionFunc(180),
	)
	if err != nil {
		return nil, err
	}
	return p.Bytes()
}

func createBarChart(data map[string]int) ([]byte, error) {
	var dates []string
	for d := range data {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	if len(dates) > 10 {
		dates = dates[len(dates)-10:]
	}

	var values [][]float64
	var labels []string
	for _, d := range dates {
		values = append(values, []float64{float64(data[d])})
		labels = append(labels, d[5:])
	}

	p, err := charts.BarRender(values,
		charts.TitleOptionFunc(charts.TitleOption{Text: ""}),
		charts.XAxisOptionFunc(charts.XAxisOption{Data: labels}),
		charts.WidthOptionFunc(240),
		charts.HeightOptionFunc(180),
	)
	if err != nil {
		return nil, err
	}
	return p.Bytes()
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
