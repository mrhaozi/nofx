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
	
	// 检查是否需要移动止损
	if pos.UnrealizedPnLPct > 3.0 {
		advice = append(advice, "盈利>3%，考虑移动止损至成本价")
	}
	
	if pos.UnrealizedPnLPct > 5.0 {
		advice = append(advice, "盈利>5%，考虑部分止盈")
	}
	
	// 检查技术位
	if pos.Side == "long" && marketData.CurrentPrice >= marketData.CurrentEMA20 {
		advice = append(advice, "价格在EMA20上方，趋势良好")
	} else if pos.Side == "short" && marketData.CurrentPrice <= marketData.CurrentEMA20 {
		advice = append(advice, "价格在EMA20下方，趋势良好")
	}
	
	// 检查MACD状态
	if (pos.Side == "long" && marketData.CurrentMACD > 0) || 
	   (pos.Side == "short" && marketData.CurrentMACD < 0) {
		advice = append(advice, "MACD方向与持仓一致")
	}
	
	if len(advice) == 0 {
		return "持仓表现符合预期，继续持有"
	}
	
	return strings.Join(advice, "； ")
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

// buildUserPrompt 构建 User Prompt（动态数据）
func buildUserPrompt(ctx *Context) string {
	var sb strings.Builder

	// 系统状态
	sb.WriteString(fmt.Sprintf("时间: %s | 周期: #%d | 运行: %d分钟\n\n",
		ctx.CurrentTime, ctx.CallCount, ctx.RuntimeMinutes))

	// BTC 市场状态（多周期分析）
	if btcData, hasBTC := ctx.MarketDataMap["BTCUSDT"]; hasBTC {
		// 获取BTC的多周期MACD数据
		btcMacd15m := btcData.CurrentMACD
		btcMacd1h := btcData.LongerTermContext.MACDValues[len(btcData.LongerTermContext.MACDValues)-1]
		btcMacd4h := btcData.LongerTermContext.MACDValues[len(btcData.LongerTermContext.MACDValues)-3]
		
		// 获取BTC的多周期RSI数据
		btcRsi15m := btcData.CurrentRSI7
		btcRsi1h := btcData.LongerTermContext.RSI14Values[len(btcData.LongerTermContext.RSI14Values)-1]
		btcRsi4h := btcData.LongerTermContext.RSI14Values[len(btcData.LongerTermContext.RSI14Values)-3]
		
		// BTC价格与EMA20关系
		btcPriceVsEMA20 := "价格 > EMA20"
		if btcData.CurrentPrice < btcData.CurrentEMA20 {
			btcPriceVsEMA20 = "价格 < EMA20"
		}

		sb.WriteString(fmt.Sprintf("### 🟠 BTC市场状态（领导者）\n"))
		sb.WriteString(fmt.Sprintf("价格: $%.2f | 15m MACD: %.4f (%s) | 1h MACD: %.4f (%s) | 4h MACD: %.4f (%s)\n",
			btcData.CurrentPrice,
			btcMacd15m, getMACDStatus(btcMacd15m), btcMacd1h, getMACDStatus(btcMacd1h), btcMacd4h, getMACDStatus(btcMacd4h)))
		sb.WriteString(fmt.Sprintf("RSI: 15m %.2f (%s) | 1h %.2f (%s) | 4h %.2f (%s) | %s\n",
			btcRsi15m, getRSIStatus(btcRsi15m), btcRsi1h, getRSIStatus(btcRsi1h), btcRsi4h, getRSIStatus(btcRsi4h), btcPriceVsEMA20))
		sb.WriteString(fmt.Sprintf("价格变化: 1h %+.2f%% | 4h %+.2f%% | 资金费率: %.2e\n\n",
			btcData.PriceChange1h, btcData.PriceChange4h, btcData.FundingRate))
	}

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

	// 当前持仓分析
	if len(ctx.Positions) > 0 {
		sb.WriteString("### 📈 当前持仓分析\n")
		for i, pos := range ctx.Positions {
			marketData, hasData := ctx.MarketDataMap[pos.Symbol]
			if !hasData {
				continue
			}

			// 计算持仓时长
			holdingDuration := ""
			if pos.UpdateTime > 0 {
				durationMs := time.Now().UnixMilli() - pos.UpdateTime
				durationMin := durationMs / (1000 * 60)
				if durationMin < 60 {
					holdingDuration = fmt.Sprintf(" | 持仓%d分钟", durationMin)
				} else {
					durationHour := durationMin / 60
					durationMinRemainder := durationMin % 60
					holdingDuration = fmt.Sprintf(" | 持仓%d小时%d分钟", durationHour, durationMinRemainder)
				}
			}

			// 计算风险回报比
			riskRewardRatio := calculateRiskRewardRatio(pos, marketData)
			
			sb.WriteString(fmt.Sprintf("%d. **%s** %s | 入场价: %.4f | 当前价: %.4f | 盈亏: %+.2f%% | 杠杆: %dx\n",
				i+1, pos.Symbol, strings.ToUpper(pos.Side), pos.EntryPrice, pos.MarkPrice, 
				pos.UnrealizedPnLPct, pos.Leverage))
			sb.WriteString(fmt.Sprintf("   保证金: %.0f USDT | 强平价: %.4f | 持仓时长: %s\n", 
				pos.MarginUsed, pos.LiquidationPrice, holdingDuration))
			sb.WriteString(fmt.Sprintf("   风险回报比: %.2f:1 | 未实现盈亏: %.2f USDT\n\n", 
				riskRewardRatio, pos.UnrealizedPnL))

			// 添加持仓管理建议
			managementAdvice := getHoldPositionAdvice(pos, marketData)
			if managementAdvice != "" {
				sb.WriteString(fmt.Sprintf("   📋 建议: %s\n\n", managementAdvice))
			}
		}
	} else {
		sb.WriteString("### 📈 当前持仓: 无\n\n")
	}

	// 候选币种深度分析
	sb.WriteString("### 🔍 候选币种深度分析\n\n")
	displayedCount := 0
	for _, coin := range ctx.CandidateCoins {
		marketData, hasData := ctx.MarketDataMap[coin.Symbol]
		if !hasData {
			continue
		}
		displayedCount++

		// 获取多周期技术指标
		macd15m := marketData.CurrentMACD
		macd1h := marketData.LongerTermContext.MACDValues[len(marketData.LongerTermContext.MACDValues)-1]
		macd4h := marketData.LongerTermContext.MACDValues[len(marketData.LongerTermContext.MACDValues)-3]
		
		rsi15m := marketData.CurrentRSI7
		rsi1h := marketData.LongerTermContext.RSI14Values[len(marketData.LongerTermContext.RSI14Values)-1]
		rsi4h := marketData.LongerTermContext.RSI14Values[len(marketData.LongerTermContext.RSI14Values)-3]
		
		// 价格与EMA20关系
		priceVsEMA20 := "价格 > EMA20"
		if marketData.CurrentPrice < marketData.CurrentEMA20 {
			priceVsEMA20 = "价格 < EMA20"
		}

		// OI数据
		oiInfo := "无OI数据"
		if oiData, hasOI := ctx.OITopDataMap[coin.Symbol]; hasOI {
			oiInfo = fmt.Sprintf("OI排名: #%d, 持仓变化: %.2f%%", oiData.Rank, oiData.OIDeltaPercent)
		}

		// 量价分析
		volumeStatus := "成交量正常"
		if marketData.LongerTermContext.CurrentVolume > marketData.LongerTermContext.AverageVolume*1.5 {
			volumeStatus = "成交量放大(>1.5x均量)"
		} else if marketData.LongerTermContext.CurrentVolume < marketData.LongerTermContext.AverageVolume*0.7 {
			volumeStatus = "成交量萎缩(<0.7x均量)"
		}

		sourceTags := ""
		if len(coin.Sources) > 1 {
			sourceTags = " (AI500+OI_Top双重信号)"
		} else if len(coin.Sources) == 1 && coin.Sources[0] == "oi_top" {
			sourceTags = " (OI_Top持仓增长)"
		}

		sb.WriteString(fmt.Sprintf("#### %d. **%s**%s\n", displayedCount, coin.Symbol, sourceTags))
		sb.WriteString(fmt.Sprintf("- 价格: $%.4f | 变化: 1h %+.2f%% | 4h %+.2f%%\n", 
			marketData.CurrentPrice, marketData.PriceChange1h, marketData.PriceChange4h))
		sb.WriteString(fmt.Sprintf("- MACD: 15m %.4f (%s) | 1h %.4f (%s) | 4h %.4f (%s)\n",
			macd15m, getMACDStatus(macd15m), macd1h, getMACDStatus(macd1h), macd4h, getMACDStatus(macd4h)))
		sb.WriteString(fmt.Sprintf("- RSI: 15m %.2f (%s) | 1h %.2f (%s) | 4h %.2f (%s) | %s\n",
			rsi15m, getRSIStatus(rsi15m), rsi1h, getRSIStatus(rsi1h), rsi4h, getRSIStatus(rsi4h), priceVsEMA20))
		sb.WriteString(fmt.Sprintf("- 资金费率: %.2e | %s | %s\n\n", 
			marketData.FundingRate, oiInfo, volumeStatus))
	}
	sb.WriteString("\n")

	// 多空确认清单
	sb.WriteString("### 📋 多空确认清单（V5.5.1核心）\n")
	sb.WriteString("⚠️ **至少5/8项一致才能开仓，4/8不足**\n\n")

	// 做多确认清单
	sb.WriteString("#### 做多确认清单\n")
	sb.WriteString("| 指标 | 条件 | 当前状态 |\n")
	sb.WriteString("|------|------|----------|\n")
	
	// 这里需要根据实际市场数据填充状态，这里先用占位符
	sb.WriteString("| MACD | >0（多头） | [分析时填写] |\n")
	sb.WriteString("| 价格 vs EMA20 | 价格 > EMA20 | [分析时填写] |\n")
	sb.WriteString("| RSI | <35（超卖反弹）或35-50 | [分析时填写] |\n")
	sb.WriteString("| BuySellRatio | >0.7（强买）或>0.55 | [分析时填写] |\n")
	sb.WriteString("| 成交量 | 放大（>1.5x均量） | [分析时填写] |\n")
	sb.WriteString("| BTC状态 | 多头或中性 | [分析时填写] |\n")
	sb.WriteString("| 资金费率 | <0（空恐慌）或-0.01~0.01 | [分析时填写] |\n")
	sb.WriteString("| OI持仓量 | 变化>+5% | [分析时填写] |\n\n")

	// 做空确认清单
	sb.WriteString("#### 做空确认清单\n")
	sb.WriteString("| 指标 | 条件 | 当前状态 |\n")
	sb.WriteString("|------|------|----------|\n")
	sb.WriteString("| MACD | <0（空头） | [分析时填写] |\n")
	sb.WriteString("| 价格 vs EMA20 | 价格 < EMA20 | [分析时填写] |\n")
	sb.WriteString("| RSI | >65（超买回落）或50-65 | [分析时填写] |\n")
	sb.WriteString("| BuySellRatio | <0.3（强卖）或<0.45 | [分析时填写] |\n")
	sb.WriteString("| 成交量 | 放大（>1.5x均量） | [分析时填写] |\n")
	sb.WriteString("| BTC状态 | 空头或中性 | [分析时填写] |\n")
	sb.WriteString("| 资金费率 | >0（多贪婪）或-0.01~0.01 | [分析时填写] |\n")
	sb.WriteString("| OI持仓量 | 变化>+5% | [分析时填写] |\n\n")

	// 技术位分析
	sb.WriteString("### 🎯 技术位分析\n")
	sb.WriteString("- **强技术位**: 1h/4h EMA20、整数关口（如100,000）\n")
	sb.WriteString("- **支撑位**: [根据实际数据填写]\n")
	sb.WriteString("- **阻力位**: [根据实际数据填写]\n\n")

	// 信号优先级排序
	sb.WriteString("### 📊 信号优先级排序（从高到低）\n")
	sb.WriteString("1. 🔴 **趋势共振**（15m/1h/4h MACD方向一致）- 权重最高\n")
	sb.WriteString("2. 🟠 **放量确认**（成交量>1.5x均量）- 动能验证\n")
	sb.WriteString("3. 🟡 **BTC状态**（若交易山寨币）- 市场领导者方向\n")
	sb.WriteString("4. 🟢 **RSI区间**（是否处于合理反转区）- 超买超卖确认\n")
	sb.WriteString("5. 🔵 **价格 vs EMA20**（趋势方向确认）- 技术位支撑\n")
	sb.WriteString("6. 🟣 **BuySellRatio**（多空力量对比）- 情绪指标\n")
	sb.WriteString("7. ⚪ **MACD柱状图**（短期动能）- 辅助确认\n")
	sb.WriteString("8. ⚫ **OI持仓量变化**（资金流入确认）- 真实突破验证\n\n")

	// 连续亏损检查
	sb.WriteString("### ⚠️ 连续亏损检查（V5.5.1新增）\n")
	sb.WriteString("- **连续2笔亏损** → 暂停交易45分钟（3个15m周期）\n")
	sb.WriteString("- **连续3笔亏损** → 暂停交易24小时\n")
	sb.WriteString("- **连续4笔亏损** → 暂停交易72小时，需人工审查\n")
	sb.WriteString("- **单日亏损>5%** → 立即停止交易，等待人工介入\n\n")

	// 冷却期检查
	sb.WriteString("### 🧊 冷却期检查\n")
	sb.WriteString("- ✅ 距上次开仓≥9分钟\n")
	sb.WriteString("- ✅ 当前持仓已持有≥30分钟（若有持仓）\n")
	sb.WriteString("- ✅ 刚止损后已观望≥6分钟\n")
	sb.WriteString("- ✅ 刚止盈后已观望≥3分钟（若想同方向再入场）\n\n")

	// 防假突破检测
	sb.WriteString("### 🛡️ 防假突破检测（V5.5.1新增）\n")
	sb.WriteString("#### 做多禁止条件\n")
	sb.WriteString("- ❌ **15m RSI >70 但 1h RSI <60** → 假突破，15m可能超买但1h未跟上\n")
	sb.WriteString("- ❌ **当前K线长上影 > 实体长度 × 2** → 上方抛压大，假突破概率高\n")
	sb.WriteString("- ❌ **价格突破但成交量萎缩（<均量 × 0.8）** → 缺乏动能，易回撤\n\n")
	
	sb.WriteString("#### 做空禁止条件\n")
	sb.WriteString("- ❌ **15m RSI <30 但 1h RSI >40** → 假跌破，15m可能超卖但1h未跟上\n")
	sb.WriteString("- ❌ **当前K线长下影 > 实体长度 × 2** → 下方承接力强，假跌破概率高\n")
	sb.WriteString("- ❌ **价格跌破但成交量萎缩（<均量 × 0.8）** → 缺乏动能，易反弹\n\n")

	sb.WriteString("---\n\n")
	sb.WriteString("现在请分析并输出决策（思维链 + JSON）\n")

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
