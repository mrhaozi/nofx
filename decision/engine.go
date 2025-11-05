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
	CurrentTime        string                     `json:"current_time"`
	RuntimeMinutes     int                        `json:"runtime_minutes"`
	CallCount          int                        `json:"call_count"`
	Account            AccountInfo                `json:"account"`
	Positions          []PositionInfo             `json:"positions"`
	CandidateCoins     []CandidateCoin            `json:"candidate_coins"`
	MarketDataMap      map[string]*market.Data    `json:"-"` // 不序列化，但内部使用
	MarketExtraDataMap map[string]*market.ExtraData `json:"-"` // 新增，用于存储K线等额外数据
	OITopDataMap       map[string]*OITopData      `json:"-"` // OI Top数据映射
	Performance        interface{}                `json:"-"` // 历史表现分析（logger.PerformanceAnalysis）
	BTCETHLeverage     int                        `json:"-"` // BTC/ETH杠杆倍数（从配置读取）
	AltcoinLeverage    int                        `json:"-"` // 山寨币杠杆倍数（从配置读取）
}

// Decision AI的交易决策
type Decision struct {
	Symbol                string  `json:"symbol"`
	Action                string  `json:"action"` // "buy_to_enter", "sell_to_enter", "close", "hold", "wait", "update_stop_loss", "update_take_profit", "partial_close"
	Leverage              int     `json:"leverage,omitempty"`
	StopLoss              float64 `json:"stop_loss,omitempty"`
	TakeProfit            float64 `json:"take_profit,omitempty"`
	Confidence            float64 `json:"confidence,omitempty"` // 信心度 (0-1)
	RiskUSD               float64 `json:"risk_usd,omitempty"`   // 最大美元风险
	InvalidationCondition string  `json:"invalidation_condition,omitempty"`
	SlippageBuffer        float64 `json:"slippage_buffer,omitempty"`
	Reasoning             string  `json:"reasoning"`
	// Fields for dynamic adjustments
	NewStopLoss     float64 `json:"new_stop_loss,omitempty"`
	NewTakeProfit   float64 `json:"new_take_profit,omitempty"`
	ClosePercentage float64 `json:"close_percentage,omitempty"` // 0-100
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
	userPrompt := buildUserPrompt(ctx)

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
	ctx.MarketExtraDataMap = make(map[string]*market.ExtraData)
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
		data, extraData, err := market.Get(symbol)
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
		ctx.MarketExtraDataMap[symbol] = extraData
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

// getMACDStatus 返回MACD状态描述
func getMACDStatus(macd float64) string {
	if macd > 0 {
		return "多头"
	} else if macd < 0 {
		return "空头"
	}
	return "零轴附近"
}

// getRSIStatus 返回RSI状态描述
func getRSIStatus(rsi float64) string {
	if rsi < 30 {
		return "超卖"
	} else if rsi > 70 {
		return "超买"
	} else if rsi < 35 {
		return "低位"
	} else if rsi > 65 {
		return "高位"
	} else if rsi < 50 {
		return "弱势"
	} else {
		return "强势"
	}
}

// calculateRiskRewardRatio 计算持仓的风险回报比
func calculateRiskRewardRatio(pos PositionInfo, marketData *market.Data) float64 {
	if pos.Side == "long" {
		// 做多：风险 = 入场价 - 强平价，回报 = 当前价 - 入场价
		risk := pos.EntryPrice - pos.LiquidationPrice
		reward := pos.MarkPrice - pos.EntryPrice
		if risk > 0 {
			return reward / risk
		}
	} else if pos.Side == "short" {
		// 做空：风险 = 强平价 - 入场价，回报 = 入场价 - 当前价
		risk := pos.LiquidationPrice - pos.EntryPrice
		reward := pos.EntryPrice - pos.MarkPrice
		if risk > 0 {
			return reward / risk
		}
	}
	return 0.0
}

// getHoldPositionAdvice 返回持仓管理建议
func getHoldPositionAdvice(pos PositionInfo, marketData *market.Data) string {
	var advice []string

	// 检查盈利情况
	if pos.UnrealizedPnLPct > 5.0 {
		advice = append(advice, "盈利>5%，考虑 partial_close(50%) 锁定利润")
	} else if pos.UnrealizedPnLPct > 3.0 {
		advice = append(advice, "盈利>3%，考虑 update_stop_loss 移至成本价")
	}

	// 检查趋势是否改变
	trendChanged := false
	if pos.Side == "long" && marketData.CurrentMACD < 0 {
		trendChanged = true
	} else if pos.Side == "short" && marketData.CurrentMACD > 0 {
		trendChanged = true
	}
	if trendChanged {
		advice = append(advice, "MACD趋势与持仓相反，考虑 close")
	}

	if len(advice) == 0 {
		return "趋势符合预期，建议 hold"
	}

	return strings.Join(advice, "；")
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
	sb.WriteString("# 风险管理协议 (强制)\n\n")
	sb.WriteString("1. **profit_target**: 最低盈亏比 2:1\n")
	sb.WriteString("2. **stop_loss**: 限制单笔亏损在账户 1-3%\n")
	sb.WriteString("3. **confidence**: <0.85 禁止开仓\n")
	sb.WriteString("4. **risk_usd**: 必须 ≤ 账户净值 × 风险预算（1.5-2.5%）\n\n")

	// 3. 输出格式 - 动态生成
	sb.WriteString("# 输出格式\n\n")
	sb.WriteString("第一步: 思维链（纯文本）\n")
	sb.WriteString("简洁分析你的思考过程\n\n")
	sb.WriteString("第二步: JSON决策数组\n\n")
	sb.WriteString("```json\n[\n")
	sb.WriteString(fmt.Sprintf("  {\"symbol\": \"BTCUSDT\", \"action\": \"sell_to_enter\", \"leverage\": %d, \"stop_loss\": 68000, \"take_profit\": 65000, \"confidence\": 0.88, \"risk_usd\": 200, \"reasoning\": \"BTC状态空头，指标一致性6/8\"},\n", btcEthLeverage))
	sb.WriteString("  {\"symbol\": \"ETHUSDT\", \"action\": \"update_stop_loss\", \"new_stop_loss\": 3500, \"reasoning\": \"盈利>3%，移动止损至成本价\"},\n")
	sb.WriteString("  {\"symbol\": \"SOLUSDT\", \"action\": \"close\", \"reasoning\": \"趋势反转，平仓离场\"}\n")
	sb.WriteString("]\n```\n\n")
	sb.WriteString("字段说明:\n")
	sb.WriteString("- `action`: buy_to_enter | sell_to_enter | close | hold | wait | update_stop_loss | update_take_profit | partial_close\n")
	sb.WriteString("- `confidence`: 0-1 (开仓必须 ≥0.85)\n")
	sb.WriteString("- 开仓时必填: leverage, stop_loss, take_profit, confidence, risk_usd\n")
	sb.WriteString("- 调整时必填: new_stop_loss / new_take_profit / close_percentage\n\n")

	return sb.String()
}

// buildUserPrompt 构建 User Prompt（动态数据）
func buildUserPrompt(ctx *Context) string {
	var sb strings.Builder

	// 系统状态
	sb.WriteString(fmt.Sprintf("时间: %s | 周期: #%d | 运行: %d分钟\n\n",
		ctx.CurrentTime, ctx.CallCount, ctx.RuntimeMinutes))

	// 账户状态
	sb.WriteString(fmt.Sprintf("### 💰 账户状态\n"))
	sb.WriteString(fmt.Sprintf("净值: %.2f USDT | 可用余额: %.2f (%.1f%%) | 总盈亏: %+.2f%%\n",
		ctx.Account.TotalEquity, ctx.Account.AvailableBalance,
		(ctx.Account.AvailableBalance/ctx.Account.TotalEquity)*100, ctx.Account.TotalPnLPct))
	sb.WriteString(fmt.Sprintf("保证金使用率: %.1f%% | 持仓数量: %d个\n\n",
		ctx.Account.MarginUsedPct, ctx.Account.PositionCount))

	// 夏普比率
	if ctx.Performance != nil {
		type PerformanceData struct {
			SharpeRatio float64 `json:"sharpe_ratio"`
		}
		var perfData PerformanceData
		if jsonData, err := json.Marshal(ctx.Performance); err == nil {
			if err := json.Unmarshal(jsonData, &perfData); err == nil {
				sb.WriteString(fmt.Sprintf("### 📊 夏普比率: %.2f\n\n", perfData.SharpeRatio))
			}
		}
	}

	// BTC 市场状态（多周期分析）
	if btcData, hasBTC := ctx.MarketDataMap["BTCUSDT"]; hasBTC {
		// 获取BTC的多周期MACD数据
		btcMacd15m := btcData.CurrentMACD
		btcMacd1h := btcData.LongerTermContext.MACDValues[len(btcData.LongerTermContext.MACDValues)-1]
		btcMacd4h := btcData.LongerTermContext.MACDValues[len(btcData.LongerTermContext.MACDValues)-3]

		// BTC价格与EMA20关系
		btcPriceVsEMA20 := "价格 > EMA20"
		if btcData.CurrentPrice < btcData.CurrentEMA20 {
			btcPriceVsEMA20 = "价格 < EMA20"
		}

		sb.WriteString(fmt.Sprintf("### 🟠 BTC 状态确认（最关键）\n"))
		sb.WriteString(fmt.Sprintf("价格: $%.2f | %s\n", btcData.CurrentPrice, btcPriceVsEMA20))
		sb.WriteString(fmt.Sprintf("- **15m MACD**: %.4f (%s)\n", btcMacd15m, getMACDStatus(btcMacd15m)))
		sb.WriteString(fmt.Sprintf("- **1h MACD**: %.4f (%s)\n", btcMacd1h, getMACDStatus(btcMacd1h)))
		sb.WriteString(fmt.Sprintf("- **4h MACD**: %.4f (%s)\n\n", btcMacd4h, getMACDStatus(btcMacd4h)))
	}

	// 当前持仓分析
	if len(ctx.Positions) > 0 {
		sb.WriteString("### 📈 评估持仓\n")
		for i, pos := range ctx.Positions {
			marketData, hasData := ctx.MarketDataMap[pos.Symbol]
			if !hasData {
				continue
			}

			// 持仓时长
			holdingDuration := ""
			if pos.UpdateTime > 0 {
				durationMin := (time.Now().UnixMilli() - pos.UpdateTime) / (1000 * 60)
				holdingDuration = fmt.Sprintf(" | 持仓%d分钟", durationMin)
			}

			sb.WriteString(fmt.Sprintf("%d. **%s** %s | 入场价: %.4f | 当前价: %.4f | 盈亏: %+.2f%%%s\n",
				i+1, pos.Symbol, strings.ToUpper(pos.Side), pos.EntryPrice, pos.MarkPrice,
				pos.UnrealizedPnLPct, holdingDuration))

			// 持仓管理建议
			managementAdvice := getHoldPositionAdvice(pos, marketData)
			sb.WriteString(fmt.Sprintf("   📋 **建议**: %s\n\n", managementAdvice))
		}
	} else {
		sb.WriteString("### 📈 评估持仓: 无\n\n")
	}

	// 候选币种深度分析
	sb.WriteString("### 🔍 评估新机会\n")
	displayedCount := 0
	for _, coin := range ctx.CandidateCoins {
		marketData, hasData := ctx.MarketDataMap[coin.Symbol]
		if !hasData {
			continue
		}
		extraData, hasExtraData := ctx.MarketExtraDataMap[coin.Symbol]
		if !hasExtraData {
			continue
		}
		displayedCount++

		// 多周期技术指标
		macd15m := marketData.CurrentMACD
		macd1h := marketData.LongerTermContext.MACDValues[len(marketData.LongerTermContext.MACDValues)-1]
		macd4h := marketData.LongerTermContext.MACDValues[len(marketData.LongerTermContext.MACDValues)-3]

		rsi15m := marketData.CurrentRSI7
		rsi1h := marketData.LongerTermContext.RSI14Values[len(marketData.LongerTermContext.RSI14Values)-1]

		// 价格与EMA20关系
		priceVsEMA20 := "价格 > EMA20"
		if marketData.CurrentPrice < marketData.CurrentEMA20 {
			priceVsEMA20 = "价格 < EMA20"
		}

		// OI数据
		oiInfo := "无OI数据"
		if oiData, hasOI := ctx.OITopDataMap[coin.Symbol]; hasOI {
			oiInfo = fmt.Sprintf("OI变化: %+.2f%%", oiData.OIDeltaPercent)
		}

		// 量价分析
		volumeStatus := "成交量正常"
		if marketData.LongerTermContext.CurrentVolume > marketData.LongerTermContext.AverageVolume*1.5 {
			volumeStatus = fmt.Sprintf("放量(%.1fx)", marketData.LongerTermContext.CurrentVolume/marketData.LongerTermContext.AverageVolume)
		} else if marketData.LongerTermContext.CurrentVolume < marketData.LongerTermContext.AverageVolume*0.8 {
			volumeStatus = fmt.Sprintf("缩量(%.1fx)", marketData.LongerTermContext.CurrentVolume/marketData.LongerTermContext.AverageVolume)
		}

		// K线形态分析 (用于防假突破)
		kline := extraData.LatestKline3m
		klineBody := math.Abs(kline.Close - kline.Open)
		klineRange := kline.High - kline.Low
		upperShadow := kline.High - math.Max(kline.Open, kline.Close)
		lowerShadow := math.Min(kline.Open, kline.Close) - kline.Low
		klineInfo := ""
		if klineRange > 0 {
			if upperShadow > klineBody*2 {
				klineInfo = " | 长上影"
			}
			if lowerShadow > klineBody*2 {
				klineInfo = " | 长下影"
			}
			if klineBody < klineRange*0.2 {
				klineInfo = " | 十字星"
			}
		}

		sb.WriteString(fmt.Sprintf("#### %d. **%s**\n", displayedCount, coin.Symbol))
		sb.WriteString(fmt.Sprintf("- **价格**: $%.4f (%s%s)\n", marketData.CurrentPrice, priceVsEMA20, klineInfo))
		sb.WriteString(fmt.Sprintf("- **趋势**: 15m MACD: %.4f (%s) | 1h MACD: %.4f (%s) | 4h MACD: %.4f (%s)\n",
			macd15m, getMACDStatus(macd15m), macd1h, getMACDStatus(macd1h), macd4h, getMACDStatus(macd4h)))
		sb.WriteString(fmt.Sprintf("- **动能**: 15m RSI: %.2f | 1h RSI: %.2f\n", rsi15m, rsi1h))
		sb.WriteString(fmt.Sprintf("- **市场**: 资金费率: %.2e | %s | %s\n\n",
			marketData.FundingRate, oiInfo, volumeStatus))
	}
	sb.WriteString("\n")
	sb.WriteString("---\n\n")
	sb.WriteString("现在请严格按照 System Prompt 中的决策流程和风险管理协议进行分析，并输出决策（思维链 + JSON）\n")

	return sb.String()
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
	for i := range decisions {
		if err := validateDecision(&decisions[i], accountEquity, btcEthLeverage, altcoinLeverage); err != nil {
			return fmt.Errorf("决策 #%d (%s %s) 验证失败: %w", i+1, decisions[i].Symbol, decisions[i].Action, err)
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
		"buy_to_enter":       true,
		"sell_to_enter":      true,
		"close":              true,
		"hold":               true,
		"wait":               true,
		"update_stop_loss":   true,
		"update_take_profit": true,
		"partial_close":      true,
	}

	if !validActions[d.Action] {
		return fmt.Errorf("无效的action: %s", d.Action)
	}

	// 开仓操作
	if d.Action == "buy_to_enter" || d.Action == "sell_to_enter" {
		maxLeverage := altcoinLeverage
		if d.Symbol == "BTCUSDT" || d.Symbol == "ETHUSDT" {
			maxLeverage = btcEthLeverage
		}

		if d.Leverage <= 0 || d.Leverage > maxLeverage {
			return fmt.Errorf("杠杆必须在1-%d之间: %d", maxLeverage, d.Leverage)
		}
		if d.StopLoss <= 0 || d.TakeProfit <= 0 {
			return fmt.Errorf("止损和止盈必须大于0")
		}
		if d.Confidence < 0.85 && d.Confidence != 0 { // Allow confidence to be omitted
			return fmt.Errorf("信心度过低(%.2f)，必须≥0.85才能开仓", d.Confidence)
		}
		if d.RiskUSD <= 0 {
			return fmt.Errorf("风险金额 risk_usd 必须 > 0")
		}
		maxRisk := accountEquity * 0.03 // 最大风险3%
		if d.RiskUSD > maxRisk {
			return fmt.Errorf("单笔风险金额 %.2f USD 过高，超过账户净值的3%% (%.2f USD)", d.RiskUSD, maxRisk)
		}

		// 验证止损止盈的合理性
		if d.Action == "buy_to_enter" {
			if d.StopLoss >= d.TakeProfit {
				return fmt.Errorf("做多时止损价必须小于止盈价")
			}
		} else { // sell_to_enter
			if d.StopLoss <= d.TakeProfit {
				return fmt.Errorf("做空时止损价必须大于止盈价")
			}
		}

		// 验证风险回报比（必须≥2:1）
		var entryPrice, risk, reward, riskRewardRatio float64
		// 估算一个可能的入场价来验证，假设在止损和止盈之间
		if d.Action == "buy_to_enter" {
			entryPrice = d.StopLoss + (d.TakeProfit-d.StopLoss)*0.1 // 假设在10%位置入场
			risk = entryPrice - d.StopLoss
			reward = d.TakeProfit - entryPrice
		} else {
			entryPrice = d.StopLoss - (d.StopLoss-d.TakeProfit)*0.1 // 假设在10%位置入场
			risk = d.StopLoss - entryPrice
			reward = entryPrice - d.TakeProfit
		}

		if risk > 0 {
			riskRewardRatio = reward / risk
		}

		if riskRewardRatio < 2.0 {
			return fmt.Errorf("风险回报比过低(%.2f:1)，必须≥2.0:1", riskRewardRatio)
		}
	}

	// 动态调整操作
	if d.Action == "update_stop_loss" && d.NewStopLoss <= 0 {
		return fmt.Errorf("update_stop_loss 时 new_stop_loss 必须 > 0")
	}
	if d.Action == "update_take_profit" && d.NewTakeProfit <= 0 {
		return fmt.Errorf("update_take_profit 时 new_take_profit 必须 > 0")
	}
	if d.Action == "partial_close" && (d.ClosePercentage <= 0 || d.ClosePercentage > 100) {
		return fmt.Errorf("partial_close 时 close_percentage 必须在 (0, 100] 之间")
	}

	return nil
}
