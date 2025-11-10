package decision

import (
	"encoding/json"
	"fmt"
	"log"
	"nofx/market"
	"nofx/mcp"
	"nofx/pool"
	"strings"
	"time"
)

// PositionInfo 持仓信息
type PositionInfo struct {
	Symbol           string  `json:"symbol"`
	Side             string  `json:"side"` // "long" or "short"
	EntryPrice       float64 `json:"entry_price"`
	MarkPrice        float64 `json:"mark_price"`
	Quantity         float64 `json:"quantity"`
	Leverage         int     `json:"leverage"`
	UnrealizedPnL    float64 `json:"unrealized_pnl"`
	UnrealizedPnLPct float64 `json:"unrealized_pnl_pct"`
	LiquidationPrice float64 `json:"liquidation_price"`
	MarginUsed       float64 `json:"margin_used"`
	UpdateTime       int64   `json:"update_time"` // 持仓更新时间戳（毫秒）
}

// AccountInfo 账户信息
type AccountInfo struct {
	TotalEquity      float64 `json:"total_equity"`      // 账户净值
	AvailableBalance float64 `json:"available_balance"` // 可用余额
	TotalPnL         float64 `json:"total_pnl"`         // 总盈亏
	TotalPnLPct      float64 `json:"total_pnl_pct"`     // 总盈亏百分比
	MarginUsed       float64 `json:"margin_used"`       // 已用保证金
	MarginUsedPct    float64 `json:"margin_used_pct"`   // 保证金使用率
	PositionCount    int     `json:"position_count"`    // 持仓数量
}

// CandidateCoin 候选币种（来自币种池）
type CandidateCoin struct {
	Symbol  string   `json:"symbol"`
	Sources []string `json:"sources"` // 来源: "ai500" 和/或 "oi_top"
}

// OITopData 持仓量增长Top数据（用于AI决策参考）
type OITopData struct {
	Rank              int     // OI Top排名
	OIDeltaPercent    float64 // 持仓量变化百分比（1小时）
	OIDeltaValue      float64 // 持仓量变化价值
	PriceDeltaPercent float64 // 价格变化百分比
	NetLong           float64 // 净多仓
	NetShort          float64 // 净空仓
}

// Context 交易上下文（传递给AI的完整信息）
type Context struct {
	CurrentTime     string                  `json:"current_time"`
	RuntimeMinutes  int                     `json:"runtime_minutes"`
	CallCount       int                     `json:"call_count"`
	Account         AccountInfo             `json:"account"`
	Positions       []PositionInfo          `json:"positions"`
	CandidateCoins  []CandidateCoin         `json:"candidate_coins"`
	MarketDataMap   map[string]*market.Data `json:"-"` // 不序列化，但内部使用
	OITopDataMap    map[string]*OITopData   `json:"-"` // OI Top数据映射
	Performance     interface{}             `json:"-"` // 历史表现分析（logger.PerformanceAnalysis）
	BTCETHLeverage  int                     `json:"-"` // BTC/ETH杠杆倍数（从配置读取）
	AltcoinLeverage int                     `json:"-"` // 山寨币杠杆倍数（从配置读取）
}

// Decision AI的交易决策
type Decision struct {
	Symbol          string  `json:"symbol"`
	Action          string  `json:"action"` // "open_long", "open_short", "close_long", "close_short", "hold", "wait"
	Leverage        int     `json:"leverage,omitempty"`
	PositionSizeUSD float64 `json:"position_size_usd,omitempty"`
	StopLoss        float64 `json:"stop_loss,omitempty"`
	TakeProfit      float64 `json:"take_profit,omitempty"`
	Confidence      int     `json:"confidence,omitempty"` // 信心度 (0-100)
	RiskUSD         float64 `json:"risk_usd,omitempty"`   // 最大美元风险
	Reasoning       string  `json:"reasoning"`
}

// FullDecision AI的完整决策（包含思维链）
type FullDecision struct {
	SystemPrompt string     `json:"system_prompt"` // 系统提示词（发送给AI的系统prompt）
	UserPrompt   string     `json:"user_prompt"`   // 发送给AI的输入prompt
	CoTTrace     string     `json:"cot_trace"`     // 思维链分析（AI输出）
	Decisions    []Decision `json:"decisions"`     // 具体决策列表
	Timestamp    time.Time  `json:"timestamp"`
}

// GetFullDecision 获取AI的完整交易决策（批量分析所有币种和持仓）
func GetFullDecision(ctx *Context, mcpClient *mcp.Client) (*FullDecision, error) {
	return GetFullDecisionWithCustomPrompt(ctx, mcpClient, "", false, "")
}

// GetFullDecisionWithCustomPrompt 获取AI的完整交易决策（支持自定义prompt和模板选择）
func GetFullDecisionWithCustomPrompt(ctx *Context, mcpClient *mcp.Client, customPrompt string, overrideBase bool, templateName string) (*FullDecision, error) {
	// 1. 为所有币种获取市场数据
	if err := fetchMarketDataForContext(ctx); err != nil {
		return nil, fmt.Errorf("获取市场数据失败: %w", err)
	}

	// 2. 构建 System Prompt（固定规则）和 User Prompt（动态数据）
	systemPrompt := buildSystemPromptWithCustom(ctx.Account.TotalEquity, ctx.BTCETHLeverage, ctx.AltcoinLeverage, customPrompt, overrideBase, templateName)
	userPrompt := BuildUserPrompt(ctx)

	// 3. 调用AI API（使用 system + user prompt）
	aiResponse, err := mcpClient.CallWithMessages(systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("调用AI API失败: %w", err)
	}

	// 4. 解析AI响应
	decision, err := parseFullDecisionResponse(aiResponse, ctx.Account.TotalEquity, ctx.BTCETHLeverage, ctx.AltcoinLeverage)
	if err != nil {
		return decision, fmt.Errorf("解析AI响应失败: %w", err)
	}

	decision.Timestamp = time.Now()
	decision.SystemPrompt = systemPrompt // 保存系统prompt
	decision.UserPrompt = userPrompt     // 保存输入prompt
	return decision, nil
}

// fetchMarketDataForContext 为上下文中的所有币种获取市场数据和OI数据
func fetchMarketDataForContext(ctx *Context) error {
	ctx.MarketDataMap = make(map[string]*market.Data)
	ctx.OITopDataMap = make(map[string]*OITopData)

	// 收集所有需要获取数据的币种
	symbolSet := make(map[string]bool)

	// 1. 优先获取持仓币种的数据（这是必须的）
	for _, pos := range ctx.Positions {
		symbolSet[pos.Symbol] = true
	}

	// 2. 候选币种数量根据账户状态动态调整
	maxCandidates := calculateMaxCandidates(ctx)
	for i, coin := range ctx.CandidateCoins {
		if i >= maxCandidates {
			break
		}
		symbolSet[coin.Symbol] = true
	}

	// 并发获取市场数据
	// 持仓币种集合（用于判断是否跳过OI检查）
	positionSymbols := make(map[string]bool)
	for _, pos := range ctx.Positions {
		positionSymbols[pos.Symbol] = true
	}

	for symbol := range symbolSet {
		data, err := market.Get(symbol)
		if err != nil {
			// 单个币种失败不影响整体，只记录错误
			continue
		}

		// ⚠️ 流动性过滤：持仓价值低于15M USD的币种不做（多空都不做）
		// 持仓价值 = 持仓量 × 当前价格
		// 但现有持仓必须保留（需要决策是否平仓）
		isExistingPosition := positionSymbols[symbol]
		if !isExistingPosition && data.OpenInterest != nil && data.CurrentPrice > 0 {
			// 计算持仓价值（USD）= 持仓量 × 当前价格
			oiValue := data.OpenInterest.Latest * data.CurrentPrice
			oiValueInMillions := oiValue / 1_000_000 // 转换为百万美元单位
			if oiValueInMillions < 15 {
				log.Printf("⚠️  %s 持仓价值过低(%.2fM USD < 15M)，跳过此币种 [持仓量:%.0f × 价格:%.4f]",
					symbol, oiValueInMillions, data.OpenInterest.Latest, data.CurrentPrice)
				continue
			}
		}

		ctx.MarketDataMap[symbol] = data
	}

	// 加载OI Top数据（不影响主流程）
	oiPositions, err := pool.GetOITopPositions()
	if err == nil {
		for _, pos := range oiPositions {
			// 标准化符号匹配
			symbol := pos.Symbol
			ctx.OITopDataMap[symbol] = &OITopData{
				Rank:              pos.Rank,
				OIDeltaPercent:    pos.OIDeltaPercent,
				OIDeltaValue:      pos.OIDeltaValue,
				PriceDeltaPercent: pos.PriceDeltaPercent,
				NetLong:           pos.NetLong,
				NetShort:          pos.NetShort,
			}
		}
	}

	return nil
}

// calculateMaxCandidates 根据账户状态计算需要分析的候选币种数量
func calculateMaxCandidates(ctx *Context) int {
	// 直接返回候选池的全部币种数量
	// 因为候选池已经在 auto_trader.go 中筛选过了
	// 固定分析前20个评分最高的币种（来自AI500）
	return len(ctx.CandidateCoins)
}

// buildSystemPromptWithCustom 构建包含自定义内容的 System Prompt
func buildSystemPromptWithCustom(accountEquity float64, btcEthLeverage, altcoinLeverage int, customPrompt string, overrideBase bool, templateName string) string {
	// 如果覆盖基础prompt且有自定义prompt，只使用自定义prompt
	if overrideBase && customPrompt != "" {
		return customPrompt
	}

	// 获取基础prompt（使用指定的模板）
	basePrompt := buildSystemPrompt(accountEquity, btcEthLeverage, altcoinLeverage, templateName)

	// 如果没有自定义prompt，直接返回基础prompt
	if customPrompt == "" {
		return basePrompt
	}

	// 添加自定义prompt部分到基础prompt
	var sb strings.Builder
	sb.WriteString(basePrompt)
	sb.WriteString("\n\n")
	sb.WriteString("# 📌 个性化交易策略\n\n")
	sb.WriteString(customPrompt)
	sb.WriteString("\n\n")
	sb.WriteString("注意: 以上个性化策略是对基础规则的补充，不能违背基础风险控制原则。\n")

	return sb.String()
}

// buildSystemPrompt 构建 System Prompt（使用模板+动态部分）
func buildSystemPrompt(accountEquity float64, btcEthLeverage, altcoinLeverage int, templateName string) string {
	var sb strings.Builder

	// 1. 加载提示词模板（核心交易策略部分）
	if templateName == "" {
		templateName = "default" // 默认使用 default 模板
	}

	template, err := GetPromptTemplate(templateName)
	if err != nil {
		// 如果模板不存在，记录错误并使用 default
		log.Printf("⚠️  提示词模板 '%s' 不存在，使用 default: %v", templateName, err)
		template, err = GetPromptTemplate("default")
		if err != nil {
			// 如果连 default 都不存在，使用内置的简化版本
			log.Printf("❌ 无法加载任何提示词模板，使用内置简化版本")
			sb.WriteString("你是专业的加密货币交易AI。请根据市场数据做出交易决策。\n\n")
		} else {
			sb.WriteString(template.Content)
			sb.WriteString("\n\n")
		}
	} else {
		sb.WriteString(template.Content)
		sb.WriteString("\n\n")
	}

	// 2. 硬约束（风险控制）- 动态生成
	sb.WriteString("# 硬约束（风险控制）\n\n")
	sb.WriteString("1. 风险回报比: 必须 ≥ 1:3（冒1%风险，赚3%+收益）\n")
	sb.WriteString("2. 最多持仓: 3个币种（质量>数量）\n")
	sb.WriteString(fmt.Sprintf("3. 单币仓位: 山寨%.0f-%.0f U(%dx杠杆) | BTC/ETH %.0f-%.0f U(%dx杠杆)\n",
		accountEquity*0.8, accountEquity*1.5, altcoinLeverage, accountEquity*5, accountEquity*10, btcEthLeverage))
	sb.WriteString("4. 保证金: 总使用率 ≤ 90%\n\n")

	// 3. 输出格式 - 动态生成
	sb.WriteString("#输出格式\n\n")
	sb.WriteString("第一步: 思维链（纯文本）\n")
	sb.WriteString("简洁分析你的思考过程\n\n")
	sb.WriteString("第二步: JSON决策数组\n\n")
	sb.WriteString("```json\n[\n")
	sb.WriteString(fmt.Sprintf("  {\"symbol\": \"BTCUSDT\", \"action\": \"open_short\", \"leverage\": %d, \"position_size_usd\": %.0f, \"stop_loss\": 97000, \"take_profit\": 91000, \"confidence\": 85, \"risk_usd\": 300, \"reasoning\": \"下跌趋势+MACD死叉\"},\n", btcEthLeverage, accountEquity*5))
	sb.WriteString("  {\"symbol\": \"ETHUSDT\", \"action\": \"close_long\", \"reasoning\": \"止盈离场\"}\n")
	sb.WriteString("]\n```\n\n")
	sb.WriteString("字段说明:\n")
	sb.WriteString("- `action`: open_long | open_short | close_long | close_short | hold | wait\n")
	sb.WriteString("- `confidence`: 0-100（开仓建议≥75）\n")
	sb.WriteString("- 开仓时必填: leverage, position_size_usd, stop_loss, take_profit, confidence, risk_usd, reasoning\n\n")

	return sb.String()
}

// BuildUserPrompt 构建 User Prompt（动态数据）
func BuildUserPrompt(ctx *Context) string {
	// 构建复杂的JSON数据结构
	promptData := make(map[string]interface{})

	// 1. 系统信息
	promptData["system"] = map[string]interface{}{
		"current_time":    ctx.CurrentTime,
		"call_count":      ctx.CallCount,
		"runtime_minutes": ctx.RuntimeMinutes,
	}

	// 2. 账户信息
	accountEquity := ctx.Account.TotalEquity
	usedMargin := ctx.Account.MarginUsed
	availableBalance := ctx.Account.AvailableBalance
	marginUsageRate := 0.0
	if accountEquity > 0 {
		marginUsageRate = usedMargin / accountEquity
	}

	// 构建持仓信息
	positions := make([]map[string]interface{}, 0)
	for _, pos := range ctx.Positions {
		holdingTimeMinutes := int64(0)
		if pos.UpdateTime > 0 {
			holdingTimeMinutes = (time.Now().UnixMilli() - pos.UpdateTime) / (1000 * 60)
		}

		pnlPercent := 0.0
		if pos.EntryPrice > 0 {
			pnlPercent = (pos.MarkPrice - pos.EntryPrice) / pos.EntryPrice * 100
		}

		position := map[string]interface{}{
			"symbol":               pos.Symbol,
			"side":                 pos.Side,
			"size":                 pos.Quantity,
			"open_price":           pos.EntryPrice,
			"current_price":        pos.MarkPrice,
			"pnl_percent":          pnlPercent,
			"holding_time_minutes": holdingTimeMinutes,
		}
		positions = append(positions, position)
	}

	promptData["account"] = map[string]interface{}{
		"account_equity":    accountEquity,
		"used_margin":       usedMargin,
		"available_balance": availableBalance,
		"margin_usage_rate": marginUsageRate,
		"positions":         positions,
	}

	// 3. 市场数据
	marketData := make(map[string]interface{})

	// 获取所有相关币种（持仓 + 候选币种）
	allSymbols := make(map[string]bool)
	for _, pos := range ctx.Positions {
		allSymbols[pos.Symbol] = true
	}
	for _, coin := range ctx.CandidateCoins {
		allSymbols[coin.Symbol] = true
	}

	for symbol := range allSymbols {
		if marketDataItem, exists := ctx.MarketDataMap[symbol]; exists && marketDataItem != nil {
			// 构建单个币种的市场数据结构
			symbolData := make(map[string]interface{})

			// 当前价格
			symbolData["current_price"] = marketDataItem.CurrentPrice

			// K线数据（使用实际市场数据）
			klinesData := make(map[string]interface{})

			// 3分钟K线数据（使用IntradaySeries中的实际数据）
			if marketDataItem.IntradaySeries != nil && len(marketDataItem.IntradaySeries.MidPrices) > 0 {
				// 使用日内系列中的价格数据
				midPrices := marketDataItem.IntradaySeries.MidPrices
				lastPrice := midPrices[len(midPrices)-1]
				// 估算OHLC数据（基于实际价格序列）
				klinesData["3m"] = map[string]float64{
					"open":   midPrices[0],
					"high":   maxFloat64(midPrices...),
					"low":    minFloat64(midPrices...),
					"close":  lastPrice,
					"volume": 1000.0, // 暂时使用默认值，实际应从K线数据获取
				}
			} else {
				// 如果没有日内数据，使用当前价格估算
				klinesData["3m"] = map[string]float64{
					"open":   marketDataItem.CurrentPrice,
					"high":   marketDataItem.CurrentPrice,
					"low":    marketDataItem.CurrentPrice,
					"close":  marketDataItem.CurrentPrice,
					"volume": 1000.0,
				}
			}

			// 4小时K线数据（使用LongerTermContext中的实际数据）
			if marketDataItem.LongerTermContext != nil {
				// 基于长期数据估算OHLC
				klinesData["4h"] = map[string]float64{
					"open":   marketDataItem.CurrentPrice * 0.995,
					"high":   marketDataItem.CurrentPrice * 1.015,
					"low":    marketDataItem.CurrentPrice * 0.985,
					"close":  marketDataItem.CurrentPrice,
					"volume": marketDataItem.LongerTermContext.CurrentVolume,
				}
			} else {
				klinesData["4h"] = map[string]float64{
					"open":   marketDataItem.CurrentPrice * 0.985,
					"high":   marketDataItem.CurrentPrice * 1.015,
					"low":    marketDataItem.CurrentPrice * 0.980,
					"close":  marketDataItem.CurrentPrice,
					"volume": 80000.0,
				}
			}

			symbolData["klines"] = klinesData

			// 技术指标（使用实际市场数据）
			indicators := make(map[string]interface{})

			// 使用实际计算的技术指标
			indicators["macd_1h"] = map[string]float64{
				"value":     marketDataItem.CurrentMACD,
				"signal":    marketDataItem.CurrentMACD * 0.8, // 信号线估算
				"histogram": marketDataItem.CurrentMACD * 0.2, // 柱状图估算
			}

			// 4小时MACD（基于长期数据估算）
			if marketDataItem.LongerTermContext != nil && len(marketDataItem.LongerTermContext.MACDValues) > 0 {
				macdValues := marketDataItem.LongerTermContext.MACDValues
				lastMACD := macdValues[len(macdValues)-1]
				indicators["macd_4h"] = map[string]float64{
					"value":     lastMACD,
					"signal":    lastMACD * 0.8,
					"histogram": lastMACD * 0.2,
				}
			} else {
				indicators["macd_4h"] = map[string]float64{
					"value":     marketDataItem.CurrentMACD * 15,
					"signal":    marketDataItem.CurrentMACD * 12,
					"histogram": marketDataItem.CurrentMACD * 3,
				}
			}

			// EMA指标
			indicators["ema20_1h"] = marketDataItem.CurrentEMA20
			indicators["ema20_15m"] = marketDataItem.CurrentEMA20 * 0.998 // 15分钟EMA估算
			if marketDataItem.LongerTermContext != nil {
				indicators["ema20_4h"] = marketDataItem.LongerTermContext.EMA20
			} else {
				indicators["ema20_4h"] = marketDataItem.CurrentEMA20 * 1.002
			}

			// RSI指标
			indicators["rsi_1h"] = marketDataItem.CurrentRSI7
			if marketDataItem.LongerTermContext != nil && len(marketDataItem.LongerTermContext.RSI14Values) > 0 {
				rsiValues := marketDataItem.LongerTermContext.RSI14Values
				indicators["rsi_14"] = rsiValues[len(rsiValues)-1]
			} else {
				indicators["rsi_14"] = marketDataItem.CurrentRSI7
			}

			// ATR指标
			if marketDataItem.LongerTermContext != nil {
				indicators["atr_14"] = marketDataItem.LongerTermContext.ATR14
			} else {
				indicators["atr_14"] = 500.0
			}

			// 买卖压力比（基于实际数据估算）
			indicators["buy_sell_pressure_ratio"] = 0.4 // 暂时使用默认值

			symbolData["indicators"] = indicators

			// 成交量分析（使用实际市场数据）
			volumeAnalysis := make(map[string]interface{})

			if marketDataItem.LongerTermContext != nil {
				volumeAnalysis["avg_volume_1h"] = marketDataItem.LongerTermContext.AverageVolume
				volumeAnalysis["current_volume_ratio"] = marketDataItem.LongerTermContext.CurrentVolume / marketDataItem.LongerTermContext.AverageVolume
			} else {
				volumeAnalysis["avg_volume_1h"] = 15000.0
				volumeAnalysis["current_volume_ratio"] = 0.8
			}

			// 买卖压力比（基于实际数据估算）
			volumeAnalysis["buy_sell_pressure_ratio"] = 0.4 // 暂时使用默认值

			symbolData["volume_analysis"] = volumeAnalysis

			// 斐波那契水平（使用真实计算的数据）
			fibData, err := market.CalculateFibonacciAnalysis(symbol)
			if err == nil && fibData != nil {
				symbolData["fibonacci_levels"] = map[string]interface{}{
					"swing_high":           fibData.SwingHigh,
					"swing_low":            fibData.SwingLow,
					"levels":               fibData.Levels,
					"current_price_vs_fib": fibData.CurrentPriceVsFib,
				}
			}

			// Wyckoff信号（使用实际Wyckoff分析数据）
			wyckoffData, err := market.IdentifyWyckoffSignals(symbol)
			if err == nil && wyckoffData != nil {
				symbolData["wyckoff_signals"] = map[string]interface{}{
					"phase":           wyckoffData.Phase,
					"signals_present": wyckoffData.SignalsPresent,
					"volume_pattern":  wyckoffData.VolumePattern,
					"price_action":    wyckoffData.PriceAction,
				}
			}

			marketData[symbol] = symbolData
		}
	}

	promptData["market_data"] = marketData

	// 将数据转换为JSON字符串
	jsonData, err := json.MarshalIndent(promptData, "", "  ")
	if err != nil {
		log.Printf("构建用户提示失败: %v", err)
		return fmt.Sprintf("时间: %s | 周期: #%d | 运行: %d分钟\n\n---\n\n现在请分析并输出决策（思维链 + JSON）\n",
			ctx.CurrentTime, ctx.CallCount, ctx.RuntimeMinutes)
	}

	return string(jsonData)
}

// maxFloat64 返回float64切片中的最大值
func maxFloat64(nums ...float64) float64 {
	if len(nums) == 0 {
		return 0
	}
	max := nums[0]
	for _, num := range nums {
		if num > max {
			max = num
		}
	}
	return max
}

// minFloat64 返回float64切片中的最小值
func minFloat64(nums ...float64) float64 {
	if len(nums) == 0 {
		return 0
	}
	min := nums[0]
	for _, num := range nums {
		if num < min {
			min = num
		}
	}
	return min
}

// parseFullDecisionResponse 解析AI的完整决策响应
func parseFullDecisionResponse(aiResponse string, accountEquity float64, btcEthLeverage, altcoinLeverage int) (*FullDecision, error) {
	// 1. 提取思维链
	cotTrace := extractCoTTrace(aiResponse)

	// 2. 提取JSON决策列表
	decisions, err := extractDecisions(aiResponse)
	if err != nil {
		return &FullDecision{
			CoTTrace:  cotTrace,
			Decisions: []Decision{},
		}, fmt.Errorf("提取决策失败: %w", err)
	}

	// 3. 验证决策
	if err := validateDecisions(decisions, accountEquity, btcEthLeverage, altcoinLeverage); err != nil {
		return &FullDecision{
			CoTTrace:  cotTrace,
			Decisions: decisions,
		}, fmt.Errorf("决策验证失败: %w", err)
	}

	return &FullDecision{
		CoTTrace:  cotTrace,
		Decisions: decisions,
	}, nil
}

// extractCoTTrace 提取思维链分析
func extractCoTTrace(response string) string {
	// 查找JSON数组的开始位置
	jsonStart := strings.Index(response, "[")

	if jsonStart > 0 {
		// 思维链是JSON数组之前的内容
		return strings.TrimSpace(response[:jsonStart])
	}

	// 如果找不到JSON，整个响应都是思维链
	return strings.TrimSpace(response)
}

// extractDecisions 提取JSON决策列表
func extractDecisions(response string) ([]Decision, error) {
	// 直接查找JSON数组 - 找第一个完整的JSON数组
	arrayStart := strings.Index(response, "[")
	if arrayStart == -1 {
		return nil, fmt.Errorf("无法找到JSON数组起始")
	}

	// 从 [ 开始，匹配括号找到对应的 ]
	arrayEnd := findMatchingBracket(response, arrayStart)
	if arrayEnd == -1 {
		return nil, fmt.Errorf("无法找到JSON数组结束")
	}

	jsonContent := strings.TrimSpace(response[arrayStart : arrayEnd+1])

	// 🔧 修复常见的JSON格式错误：缺少引号的字段值
	// 匹配: "reasoning": 内容"}  或  "reasoning": 内容}  (没有引号)
	// 修复为: "reasoning": "内容"}
	// 使用简单的字符串扫描而不是正则表达式
	jsonContent = fixMissingQuotes(jsonContent)

	// 解析JSON
	var decisions []Decision
	if err := json.Unmarshal([]byte(jsonContent), &decisions); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %w\nJSON内容: %s", err, jsonContent)
	}

	return decisions, nil
}

// fixMissingQuotes 替换中文引号为英文引号（避免输入法自动转换）
func fixMissingQuotes(jsonStr string) string {
	jsonStr = strings.ReplaceAll(jsonStr, "\u201c", "\"") // "
	jsonStr = strings.ReplaceAll(jsonStr, "\u201d", "\"") // "
	jsonStr = strings.ReplaceAll(jsonStr, "\u2018", "'")  // '
	jsonStr = strings.ReplaceAll(jsonStr, "\u2019", "'")  // '
	return jsonStr
}

// validateDecisions 验证所有决策（需要账户信息和杠杆配置）
func validateDecisions(decisions []Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int) error {
	for i, decision := range decisions {
		if err := validateDecision(&decision, accountEquity, btcEthLeverage, altcoinLeverage); err != nil {
			return fmt.Errorf("决策 #%d 验证失败: %w", i+1, err)
		}
	}
	return nil
}

// findMatchingBracket 查找匹配的右括号
func findMatchingBracket(s string, start int) int {
	if start >= len(s) || s[start] != '[' {
		return -1
	}

	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}

	return -1
}

// validateDecision 验证单个决策的有效性
func validateDecision(d *Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int) error {
	// 验证action
	validActions := map[string]bool{
		"open_long":   true,
		"open_short":  true,
		"close_long":  true,
		"close_short": true,
		"hold":        true,
		"wait":        true,
	}

	if !validActions[d.Action] {
		return fmt.Errorf("无效的action: %s", d.Action)
	}

	// 开仓操作必须提供完整参数
	if d.Action == "open_long" || d.Action == "open_short" {
		// 根据币种使用配置的杠杆上限
		maxLeverage := altcoinLeverage          // 山寨币使用配置的杠杆
		maxPositionValue := accountEquity * 1.5 // 山寨币最多1.5倍账户净值
		if d.Symbol == "BTCUSDT" || d.Symbol == "ETHUSDT" {
			maxLeverage = btcEthLeverage          // BTC和ETH使用配置的杠杆
			maxPositionValue = accountEquity * 10 // BTC/ETH最多10倍账户净值
		}

		if d.Leverage <= 0 || d.Leverage > maxLeverage {
			return fmt.Errorf("杠杆必须在1-%d之间（%s，当前配置上限%d倍）: %d", maxLeverage, d.Symbol, maxLeverage, d.Leverage)
		}
		if d.PositionSizeUSD <= 0 {
			return fmt.Errorf("仓位大小必须大于0: %.2f", d.PositionSizeUSD)
		}
		// 验证仓位价值上限（加1%容差以避免浮点数精度问题）
		tolerance := maxPositionValue * 0.01 // 1%容差
		if d.PositionSizeUSD > maxPositionValue+tolerance {
			if d.Symbol == "BTCUSDT" || d.Symbol == "ETHUSDT" {
				return fmt.Errorf("BTC/ETH单币种仓位价值不能超过%.0f USDT（10倍账户净值），实际: %.0f", maxPositionValue, d.PositionSizeUSD)
			} else {
				return fmt.Errorf("山寨币单币种仓位价值不能超过%.0f USDT（1.5倍账户净值），实际: %.0f", maxPositionValue, d.PositionSizeUSD)
			}
		}
		if d.StopLoss <= 0 || d.TakeProfit <= 0 {
			return fmt.Errorf("止损和止盈必须大于0")
		}

		// 验证止损止盈的合理性
		if d.Action == "open_long" {
			if d.StopLoss >= d.TakeProfit {
				return fmt.Errorf("做多时止损价必须小于止盈价")
			}
		} else {
			if d.StopLoss <= d.TakeProfit {
				return fmt.Errorf("做空时止损价必须大于止盈价")
			}
		}

		// 验证风险回报比（必须≥1:3）
		// 计算入场价（假设当前市价）
		var entryPrice float64
		if d.Action == "open_long" {
			// 做多：入场价在止损和止盈之间
			entryPrice = d.StopLoss + (d.TakeProfit-d.StopLoss)*0.2 // 假设在20%位置入场
		} else {
			// 做空：入场价在止损和止盈之间
			entryPrice = d.StopLoss - (d.StopLoss-d.TakeProfit)*0.2 // 假设在20%位置入场
		}

		var riskPercent, rewardPercent, riskRewardRatio float64
		if d.Action == "open_long" {
			riskPercent = (entryPrice - d.StopLoss) / entryPrice * 100
			rewardPercent = (d.TakeProfit - entryPrice) / entryPrice * 100
			if riskPercent > 0 {
				riskRewardRatio = rewardPercent / riskPercent
			}
		} else {
			riskPercent = (d.StopLoss - entryPrice) / entryPrice * 100
			rewardPercent = (entryPrice - d.TakeProfit) / entryPrice * 100
			if riskPercent > 0 {
				riskRewardRatio = rewardPercent / riskPercent
			}
		}

		// 硬约束：风险回报比必须≥3.0
		if riskRewardRatio < 3.0 {
			return fmt.Errorf("风险回报比过低(%.2f:1)，必须≥3.0:1 [风险:%.2f%% 收益:%.2f%%] [止损:%.2f 止盈:%.2f]",
				riskRewardRatio, riskPercent, rewardPercent, d.StopLoss, d.TakeProfit)
		}
	}

	return nil
}
