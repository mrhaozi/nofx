package market

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
)

// Get 获取指定代币的市场数据
func Get(symbol string) (*Data, error) {
	var klines3m, klines4h []Kline
	var err error
	// 标准化symbol
	symbol = Normalize(symbol)
	// 获取3分钟K线数据 (最近10个)
	klines3m, err = WSMonitorCli.GetCurrentKlines(symbol, "3m") // 多获取一些用于计算
	if err != nil {
		return nil, fmt.Errorf("获取3分钟K线失败: %v", err)
	}

	// 获取4小时K线数据 (最近10个)
	klines4h, err = WSMonitorCli.GetCurrentKlines(symbol, "4h") // 多获取用于计算指标
	if err != nil {
		return nil, fmt.Errorf("获取4小时K线失败: %v", err)
	}

	// 计算当前指标 (基于3分钟最新数据)
	currentPrice := klines3m[len(klines3m)-1].Close
	currentEMA20 := calculateEMA(klines3m, 20)
	currentMACD := calculateMACD(klines3m)
	currentRSI7 := calculateRSI(klines3m, 7)

	// 计算价格变化百分比
	// 1小时价格变化 = 20个3分钟K线前的价格
	priceChange1h := 0.0
	if len(klines3m) >= 21 { // 至少需要21根K线 (当前 + 20根前)
		price1hAgo := klines3m[len(klines3m)-21].Close
		if price1hAgo > 0 {
			priceChange1h = ((currentPrice - price1hAgo) / price1hAgo) * 100
		}
	}

	// 4小时价格变化 = 1个4小时K线前的价格
	priceChange4h := 0.0
	if len(klines4h) >= 2 {
		price4hAgo := klines4h[len(klines4h)-2].Close
		if price4hAgo > 0 {
			priceChange4h = ((currentPrice - price4hAgo) / price4hAgo) * 100
		}
	}

	// 获取OI数据
	oiData, err := getOpenInterestData(symbol)
	if err != nil {
		// OI失败不影响整体,使用默认值
		oiData = &OIData{Latest: 0, Average: 0}
	}

	// 获取Funding Rate
	fundingRate, _ := getFundingRate(symbol)

	// 计算日内系列数据
	intradayData := calculateIntradaySeries(klines3m)

	// 计算长期数据
	longerTermData := calculateLongerTermData(klines4h)

	return &Data{
		Symbol:            symbol,
		CurrentPrice:      currentPrice,
		PriceChange1h:     priceChange1h,
		PriceChange4h:     priceChange4h,
		CurrentEMA20:      currentEMA20,
		CurrentMACD:       currentMACD,
		CurrentRSI7:       currentRSI7,
		OpenInterest:      oiData,
		FundingRate:       fundingRate,
		IntradaySeries:    intradayData,
		LongerTermContext: longerTermData,
	}, nil
}

// calculateEMA 计算EMA
func calculateEMA(klines []Kline, period int) float64 {
	if len(klines) < period {
		return 0
	}

	// 计算SMA作为初始EMA
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += klines[i].Close
	}
	ema := sum / float64(period)

	// 计算EMA
	multiplier := 2.0 / float64(period+1)
	for i := period; i < len(klines); i++ {
		ema = (klines[i].Close-ema)*multiplier + ema
	}

	return ema
}

// calculateMACD 计算MACD
func calculateMACD(klines []Kline) float64 {
	if len(klines) < 26 {
		return 0
	}

	// 计算12期和26期EMA
	ema12 := calculateEMA(klines, 12)
	ema26 := calculateEMA(klines, 26)

	// MACD = EMA12 - EMA26
	return ema12 - ema26
}

// calculateRSI 计算RSI
func calculateRSI(klines []Kline, period int) float64 {
	if len(klines) <= period {
		return 0
	}

	gains := 0.0
	losses := 0.0

	// 计算初始平均涨跌幅
	for i := 1; i <= period; i++ {
		change := klines[i].Close - klines[i-1].Close
		if change > 0 {
			gains += change
		} else {
			losses += -change
		}
	}

	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	// 使用Wilder平滑方法计算后续RSI
	for i := period + 1; i < len(klines); i++ {
		change := klines[i].Close - klines[i-1].Close
		if change > 0 {
			avgGain = (avgGain*float64(period-1) + change) / float64(period)
			avgLoss = (avgLoss * float64(period-1)) / float64(period)
		} else {
			avgGain = (avgGain * float64(period-1)) / float64(period)
			avgLoss = (avgLoss*float64(period-1) + (-change)) / float64(period)
		}
	}

	if avgLoss == 0 {
		return 100
	}

	rs := avgGain / avgLoss
	rsi := 100 - (100 / (1 + rs))

	return rsi
}

// calculateATR 计算ATR
func calculateATR(klines []Kline, period int) float64 {
	if len(klines) <= period {
		return 0
	}

	trs := make([]float64, len(klines))
	for i := 1; i < len(klines); i++ {
		high := klines[i].High
		low := klines[i].Low
		prevClose := klines[i-1].Close

		tr1 := high - low
		tr2 := math.Abs(high - prevClose)
		tr3 := math.Abs(low - prevClose)

		trs[i] = math.Max(tr1, math.Max(tr2, tr3))
	}

	// 计算初始ATR
	sum := 0.0
	for i := 1; i <= period; i++ {
		sum += trs[i]
	}
	atr := sum / float64(period)

	// Wilder平滑
	for i := period + 1; i < len(klines); i++ {
		atr = (atr*float64(period-1) + trs[i]) / float64(period)
	}

	return atr
}

// calculateIntradaySeries 计算日内系列数据
func calculateIntradaySeries(klines []Kline) *IntradayData {
	data := &IntradayData{
		MidPrices:   make([]float64, 0, 10),
		EMA20Values: make([]float64, 0, 10),
		MACDValues:  make([]float64, 0, 10),
		RSI7Values:  make([]float64, 0, 10),
		RSI14Values: make([]float64, 0, 10),
	}

	// 获取最近10个数据点
	start := len(klines) - 10
	if start < 0 {
		start = 0
	}

	for i := start; i < len(klines); i++ {
		data.MidPrices = append(data.MidPrices, klines[i].Close)

		// 计算每个点的EMA20
		if i >= 19 {
			ema20 := calculateEMA(klines[:i+1], 20)
			data.EMA20Values = append(data.EMA20Values, ema20)
		}

		// 计算每个点的MACD
		if i >= 25 {
			macd := calculateMACD(klines[:i+1])
			data.MACDValues = append(data.MACDValues, macd)
		}

		// 计算每个点的RSI
		if i >= 7 {
			rsi7 := calculateRSI(klines[:i+1], 7)
			data.RSI7Values = append(data.RSI7Values, rsi7)
		}
		if i >= 14 {
			rsi14 := calculateRSI(klines[:i+1], 14)
			data.RSI14Values = append(data.RSI14Values, rsi14)
		}
	}

	return data
}

// calculateLongerTermData 计算长期数据
func calculateLongerTermData(klines []Kline) *LongerTermData {
	data := &LongerTermData{
		MACDValues:  make([]float64, 0, 10),
		RSI14Values: make([]float64, 0, 10),
	}

	// 计算EMA
	data.EMA20 = calculateEMA(klines, 20)
	data.EMA50 = calculateEMA(klines, 50)

	// 计算ATR
	data.ATR3 = calculateATR(klines, 3)
	data.ATR14 = calculateATR(klines, 14)

	// 计算成交量
	if len(klines) > 0 {
		data.CurrentVolume = klines[len(klines)-1].Volume
		// 计算平均成交量
		sum := 0.0
		for _, k := range klines {
			sum += k.Volume
		}
		data.AverageVolume = sum / float64(len(klines))
	}

	// 计算MACD和RSI序列
	start := len(klines) - 10
	if start < 0 {
		start = 0
	}

	for i := start; i < len(klines); i++ {
		if i >= 25 {
			macd := calculateMACD(klines[:i+1])
			data.MACDValues = append(data.MACDValues, macd)
		}
		if i >= 14 {
			rsi14 := calculateRSI(klines[:i+1], 14)
			data.RSI14Values = append(data.RSI14Values, rsi14)
		}
	}

	return data
}

// getOpenInterestData 获取OI数据
func getOpenInterestData(symbol string) (*OIData, error) {
	url := fmt.Sprintf("https://fapi.binance.com/fapi/v1/openInterest?symbol=%s", symbol)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		OpenInterest string `json:"openInterest"`
		Symbol       string `json:"symbol"`
		Time         int64  `json:"time"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	oi, _ := strconv.ParseFloat(result.OpenInterest, 64)

	return &OIData{
		Latest:  oi,
		Average: oi * 0.999, // 近似平均值
	}, nil
}

// getFundingRate 获取资金费率
func getFundingRate(symbol string) (float64, error) {
	url := fmt.Sprintf("https://fapi.binance.com/fapi/v1/premiumIndex?symbol=%s", symbol)

	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result struct {
		Symbol          string `json:"symbol"`
		MarkPrice       string `json:"markPrice"`
		IndexPrice      string `json:"indexPrice"`
		LastFundingRate string `json:"lastFundingRate"`
		NextFundingTime int64  `json:"nextFundingTime"`
		InterestRate    string `json:"interestRate"`
		Time            int64  `json:"time"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	rate, _ := strconv.ParseFloat(result.LastFundingRate, 64)
	return rate, nil
}

// Format 格式化输出市场数据
func Format(data *Data) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("current_price = %.2f, current_ema20 = %.3f, current_macd = %.3f, current_rsi (7 period) = %.3f\n\n",
		data.CurrentPrice, data.CurrentEMA20, data.CurrentMACD, data.CurrentRSI7))

	sb.WriteString(fmt.Sprintf("In addition, here is the latest %s open interest and funding rate for perps:\n\n",
		data.Symbol))

	if data.OpenInterest != nil {
		sb.WriteString(fmt.Sprintf("Open Interest: Latest: %.2f Average: %.2f\n\n",
			data.OpenInterest.Latest, data.OpenInterest.Average))
	}

	sb.WriteString(fmt.Sprintf("Funding Rate: %.2e\n\n", data.FundingRate))

	if data.IntradaySeries != nil {
		sb.WriteString("Intraday series (3‑minute intervals, oldest → latest):\n\n")

		if len(data.IntradaySeries.MidPrices) > 0 {
			sb.WriteString(fmt.Sprintf("Mid prices: %s\n\n", formatFloatSlice(data.IntradaySeries.MidPrices)))
		}

		if len(data.IntradaySeries.EMA20Values) > 0 {
			sb.WriteString(fmt.Sprintf("EMA indicators (20‑period): %s\n\n", formatFloatSlice(data.IntradaySeries.EMA20Values)))
		}

		if len(data.IntradaySeries.MACDValues) > 0 {
			sb.WriteString(fmt.Sprintf("MACD indicators: %s\n\n", formatFloatSlice(data.IntradaySeries.MACDValues)))
		}

		if len(data.IntradaySeries.RSI7Values) > 0 {
			sb.WriteString(fmt.Sprintf("RSI indicators (7‑Period): %s\n\n", formatFloatSlice(data.IntradaySeries.RSI7Values)))
		}

		if len(data.IntradaySeries.RSI14Values) > 0 {
			sb.WriteString(fmt.Sprintf("RSI indicators (14‑Period): %s\n\n", formatFloatSlice(data.IntradaySeries.RSI14Values)))
		}
	}

	if data.LongerTermContext != nil {
		sb.WriteString("Longer‑term context (4‑hour timeframe):\n\n")

		sb.WriteString(fmt.Sprintf("20‑Period EMA: %.3f vs. 50‑Period EMA: %.3f\n\n",
			data.LongerTermContext.EMA20, data.LongerTermContext.EMA50))

		sb.WriteString(fmt.Sprintf("3‑Period ATR: %.3f vs. 14‑Period ATR: %.3f\n\n",
			data.LongerTermContext.ATR3, data.LongerTermContext.ATR14))

		sb.WriteString(fmt.Sprintf("Current Volume: %.3f vs. Average Volume: %.3f\n\n",
			data.LongerTermContext.CurrentVolume, data.LongerTermContext.AverageVolume))

		if len(data.LongerTermContext.MACDValues) > 0 {
			sb.WriteString(fmt.Sprintf("MACD indicators: %s\n\n", formatFloatSlice(data.LongerTermContext.MACDValues)))
		}

		if len(data.LongerTermContext.RSI14Values) > 0 {
			sb.WriteString(fmt.Sprintf("RSI indicators (14‑Period): %s\n\n", formatFloatSlice(data.LongerTermContext.RSI14Values)))
		}
	}

	return sb.String()
}

// formatFloatSlice 格式化float64切片为字符串
func formatFloatSlice(values []float64) string {
	strValues := make([]string, len(values))
	for i, v := range values {
		strValues[i] = fmt.Sprintf("%.3f", v)
	}
	return "[" + strings.Join(strValues, ", ") + "]"
}

// Normalize 标准化symbol,确保是USDT交易对
func Normalize(symbol string) string {
	symbol = strings.ToUpper(symbol)
	if strings.HasSuffix(symbol, "USDT") {
		return symbol
	}
	return symbol + "USDT"
}

// CalculateFibonacciAnalysis 计算斐波那契分析所需波段数据
func CalculateFibonacciAnalysis(symbol string) (*FibonacciData, error) {
	// 获取4小时K线数据用于波段分析
	klines4h, err := WSMonitorCli.GetCurrentKlines(symbol, "4h")
	if err != nil {
		return nil, fmt.Errorf("获取4小时K线失败: %v", err)
	}

	if len(klines4h) < 30 { // 至少需要30根K线进行可靠的波段分析
		return nil, fmt.Errorf("K线数据不足，需要至少30根4小时K线")
	}

	// 识别波段高低点
	swingHigh, swingLow := identifySwingPoints(klines4h)

	// 计算当前价格
	currentPrice := klines4h[len(klines4h)-1].Close

	// 计算斐波那契回撤位
	levels := calculateFibonacciLevels(swingHigh, swingLow)

	// 判断当前价格位置
	currentPriceVsFib := analyzePricePosition(currentPrice, levels)

	return &FibonacciData{
		SwingHigh:         swingHigh,
		SwingLow:          swingLow,
		Levels:            levels,
		CurrentPriceVsFib: currentPriceVsFib,
	}, nil
}

// identifySwingPoints 识别波段高低点
func identifySwingPoints(klines []Kline) (float64, float64) {
	if len(klines) < 10 {
		return 0, 0
	}

	// 使用最近20根K线来识别波段高低点
	recentKlines := klines[len(klines)-20:]

	swingHigh := 0.0
	swingLow := 999999999.0

	// 寻找最高点作为波段高点
	for _, kline := range recentKlines {
		if kline.High > swingHigh {
			swingHigh = kline.High
		}
		if kline.Low < swingLow {
			swingLow = kline.Low
		}
	}

	return swingHigh, swingLow
}

// calculateFibonacciLevels 计算斐波那契回撤位
func calculateFibonacciLevels(swingHigh, swingLow float64) map[string]float64 {
	levels := make(map[string]float64)

	if swingHigh <= swingLow {
		return levels
	}

	diff := swingHigh - swingLow

	// 标准斐波那契回撤位
	fibRatios := map[string]float64{
		"23.6": 0.236,
		"38.2": 0.382,
		"50.0": 0.500,
		"61.8": 0.618,
		"70.5": 0.705,
		"78.6": 0.786,
	}

	for level, ratio := range fibRatios {
		levels[level] = swingHigh - (diff * ratio)
	}

	return levels
}

// analyzePricePosition 分析当前价格相对于斐波那契区间的位置
func analyzePricePosition(currentPrice float64, levels map[string]float64) string {
	if len(levels) == 0 {
		return "数据不足"
	}

	// 获取关键水平
	oteLower := levels["61.8"] // OTE下限
	oteUpper := levels["70.5"] // OTE上限

	if oteLower == 0 || oteUpper == 0 {
		return "数据不足"
	}

	// 判断当前价格位置
	if currentPrice >= oteLower && currentPrice <= oteUpper {
		return "在OTE区间内"
	} else if currentPrice > oteUpper {
		return "在OTE区间上方"
	} else if currentPrice < oteLower {
		return "在OTE区间下方"
	}

	// 更详细的分析
	if currentPrice >= levels["38.2"] && currentPrice <= levels["61.8"] {
		return "在斐波那契回撤区间内"
	} else if currentPrice > levels["23.6"] {
		return "在强势区域"
	} else if currentPrice < levels["78.6"] {
		return "在弱势区域"
	}

	return "在标准区域"
}

// IdentifyWyckoffSignals 识别维科夫信号
func IdentifyWyckoffSignals(symbol string) (*WyckoffSignalData, error) {
	// 获取4小时K线数据用于维科夫分析
	klines4h, err := WSMonitorCli.GetCurrentKlines(symbol, "4h")
	if err != nil {
		return nil, fmt.Errorf("获取4小时K线失败: %v", err)
	}

	if len(klines4h) < 20 { // 至少需要20根K线进行维科夫分析
		return nil, fmt.Errorf("K线数据不足，需要至少20根4小时K线")
	}

	// 识别市场阶段
	phase := identifyMarketPhase(klines4h)

	// 检测维科夫信号
	signals := detectWyckoffSignals(klines4h)

	// 分析成交量模式
	volumePattern := analyzeVolumePattern(klines4h)

	// 识别价格行为
	priceAction := identifyPriceAction(klines4h)

	return &WyckoffSignalData{
		Phase:          phase,
		SignalsPresent: signals,
		VolumePattern:  volumePattern,
		PriceAction:    priceAction,
	}, nil
}

// identifyMarketPhase 识别市场阶段
func identifyMarketPhase(klines []Kline) string {
	if len(klines) < 10 {
		return "consolidation"
	}

	// 获取最近的价格数据
	recentKlines := klines[len(klines)-10:]
	currentPrice := recentKlines[len(recentKlines)-1].Close

	// 计算价格变化趋势
	priceChanges := make([]float64, len(recentKlines)-1)
	for i := 1; i < len(recentKlines); i++ {
		change := (recentKlines[i].Close - recentKlines[i-1].Close) / recentKlines[i-1].Close * 100
		priceChanges[i-1] = change
	}

	// 计算平均价格变化
	avgChange := 0.0
	for _, change := range priceChanges {
		avgChange += change
	}
	avgChange = avgChange / float64(len(priceChanges))

	// 计算价格波动率
	volatility := calculateVolatility(recentKlines)

	// 识别阶段
	if volatility < 2.0 && math.Abs(avgChange) < 1.0 {
		// 低波动率，价格在一定范围内震荡
		return "consolidation"
	} else if avgChange > 2.0 {
		// 明显的上升趋势
		return "uptrend"
	} else if avgChange < -2.0 {
		// 明显的下降趋势
		return "downtrend"
	}

	// 进一步分析积累/分布阶段
	high := 0.0
	low := 999999999.0
	totalVolume := 0.0
	for _, kline := range recentKlines {
		if kline.High > high {
			high = kline.High
		}
		if kline.Low < low {
			low = kline.Low
		}
		totalVolume += kline.Volume
	}
	avgVolume := totalVolume / float64(len(recentKlines))

	// 计算价格位置（在区间中的位置）
	priceRange := high - low
	if priceRange <= 0 {
		return "consolidation"
	}
	positionInRange := (currentPrice - low) / priceRange

	// 基于位置判断积累或分布
	if positionInRange < 0.3 && avgVolume > 0 {
		// 价格区间低位，可能是积累阶段
		return "accumulation"
	} else if positionInRange > 0.7 && avgVolume > 0 {
		// 价格区间高位，可能是分布阶段
		return "distribution"
	}

	return "consolidation"
}

// detectWyckoffSignals 检测维科夫信号
func detectWyckoffSignals(klines []Kline) []string {
	signals := make([]string, 0)

	if len(klines) < 5 {
		return signals
	}

	// 获取最近几根K线进行分析
	recentKlines := klines[len(klines)-5:]

	// 检测Spring（假跌破）
	if isSpringPattern(recentKlines) {
		signals = append(signals, "Spring")
	}

	// 检测UTAD（假突破）
	if isUTADPattern(recentKlines) {
		signals = append(signals, "UTAD")
	}

	// 检测SOS（强势突破）
	if isSOSPattern(recentKlines) {
		signals = append(signals, "SOS")
	}

	// 检测SOW（弱势跌破）
	if isSOWPattern(recentKlines) {
		signals = append(signals, "SOW")
	}

	// 检测CLIMAX（高潮）
	if isClimaxPattern(recentKlines) {
		signals = append(signals, "CLIMAX")
	}

	// 检测TEST（测试关键位）
	if isTestPattern(recentKlines) {
		signals = append(signals, "TEST")
	}

	// 检测BREAKOUT（突破）
	if isBreakoutPattern(recentKlines) {
		signals = append(signals, "BREAKOUT")
	}

	// 检测BREAKDOWN（跌破）
	if isBreakdownPattern(recentKlines) {
		signals = append(signals, "BREAKDOWN")
	}

	return signals
}

// 维科夫信号检测辅助函数
func isSpringPattern(klines []Kline) bool {
	if len(klines) < 3 {
		return false
	}

	// Spring模式：价格短暂跌破支撑位后快速反弹
	current := klines[len(klines)-1]
	previous := klines[len(klines)-2]

	// 检查是否出现下影线较长的K线，且收盘价回到支撑位上方
	lowerShadow := previous.Close - previous.Low
	body := math.Abs(previous.Close - previous.Open)

	if lowerShadow > body*2 && current.Close > previous.Close {
		return true
	}

	return false
}

func isUTADPattern(klines []Kline) bool {
	if len(klines) < 3 {
		return false
	}

	// UTAD模式：价格短暂突破阻力位后快速回落
	current := klines[len(klines)-1]
	previous := klines[len(klines)-2]

	// 检查是否出现上影线较长的K线，且收盘价回到阻力位下方
	upperShadow := previous.High - previous.Close
	body := math.Abs(previous.Close - previous.Open)

	if upperShadow > body*2 && current.Close < previous.Close {
		return true
	}

	return false
}

func isSOSPattern(klines []Kline) bool {
	if len(klines) < 2 {
		return false
	}

	// SOS模式：强势突破，伴随着成交量放大
	current := klines[len(klines)-1]

	// 检查是否出现大阳线突破
	if current.Close > current.Open &&
		(current.Close-current.Open) > (current.High-current.Low)*0.6 {
		return true
	}

	return false
}

func isSOWPattern(klines []Kline) bool {
	if len(klines) < 2 {
		return false
	}

	// SOW模式：弱势跌破，伴随着成交量放大
	current := klines[len(klines)-1]

	// 检查是否出现大阴线跌破
	if current.Close < current.Open &&
		(current.Open-current.Close) > (current.High-current.Low)*0.6 {
		return true
	}

	return false
}

func isClimaxPattern(klines []Kline) bool {
	if len(klines) < 3 {
		return false
	}

	// CLIMAX模式：高潮，价格剧烈波动伴随巨量
	current := klines[len(klines)-1]
	volatility := (current.High - current.Low) / current.Open * 100

	// 检查是否出现极端波动（波动率超过5%）
	if volatility > 5.0 {
		return true
	}

	return false
}

func isTestPattern(klines []Kline) bool {
	if len(klines) < 2 {
		return false
	}

	// TEST模式：测试关键支撑/阻力位
	current := klines[len(klines)-1]

	// 检查是否出现小实体K线，表示测试关键位
	body := math.Abs(current.Close - current.Open)
	totalRange := current.High - current.Low

	if totalRange > 0 && body/totalRange < 0.3 {
		return true
	}

	return false
}

func isBreakoutPattern(klines []Kline) bool {
	if len(klines) < 3 {
		return false
	}

	// BREAKOUT模式：突破前期高点
	current := klines[len(klines)-1]
	previousHigh := 0.0

	for i := 0; i < len(klines)-1; i++ {
		if klines[i].High > previousHigh {
			previousHigh = klines[i].High
		}
	}

	// 检查当前K线是否突破前期高点
	if current.Close > previousHigh {
		return true
	}

	return false
}

func isBreakdownPattern(klines []Kline) bool {
	if len(klines) < 3 {
		return false
	}

	// BREAKDOWN模式：跌破前期低点
	current := klines[len(klines)-1]
	previousLow := klines[0].Low

	for i := 1; i < len(klines)-1; i++ {
		if klines[i].Low < previousLow {
			previousLow = klines[i].Low
		}
	}

	// 检查当前K线是否跌破前期低点
	if current.Close < previousLow {
		return true
	}

	return false
}

// analyzeVolumePattern 分析成交量模式
func analyzeVolumePattern(klines []Kline) string {
	if len(klines) < 5 {
		return "normal_volume"
	}

	// 获取最近几根K线
	recentKlines := klines[len(klines)-5:]

	// 计算平均成交量
	avgVolume := 0.0
	for _, kline := range recentKlines {
		avgVolume += kline.Volume
	}
	avgVolume = avgVolume / float64(len(recentKlines))

	// 获取历史平均成交量（更长期）
	historicalAvgVolume := 0.0
	start := len(klines) - 20
	if start < 0 {
		start = 0
	}

	historicalKlines := klines[start : len(klines)-5]
	if len(historicalKlines) > 0 {
		for _, kline := range historicalKlines {
			historicalAvgVolume += kline.Volume
		}
		historicalAvgVolume = historicalAvgVolume / float64(len(historicalKlines))
	}

	// 分析当前成交量
	currentVolume := recentKlines[len(recentKlines)-1].Volume

	if historicalAvgVolume > 0 {
		volumeRatio := currentVolume / historicalAvgVolume

		if volumeRatio > 2.0 {
			return "high_volume"
		} else if volumeRatio < 0.5 {
			return "low_volume"
		}
	}

	// 检查量价背离
	priceChange := (recentKlines[len(recentKlines)-1].Close - recentKlines[0].Open) / recentKlines[0].Open * 100
	volumeChange := (currentVolume - avgVolume) / avgVolume * 100

	if math.Abs(priceChange) > 2.0 && math.Abs(volumeChange) < 1.0 {
		return "divergence"
	}

	return "normal_volume"
}

// identifyPriceAction 识别价格行为
func identifyPriceAction(klines []Kline) string {
	if len(klines) < 3 {
		return "consolidation"
	}

	// 获取最近几根K线
	recentKlines := klines[len(klines)-3:]

	// 计算价格变化
	totalChange := (recentKlines[len(recentKlines)-1].Close - recentKlines[0].Open) / recentKlines[0].Open * 100

	// 计算波动率
	volatility := calculateVolatility(recentKlines)

	if math.Abs(totalChange) > 3.0 {
		if totalChange > 0 {
			return "breakout"
		} else {
			return "breakdown"
		}
	}

	if volatility > 2.0 {
		return "false_move"
	}

	if volatility < 1.0 {
		return "consolidation"
	}

	return "trending"
}

// calculateVolatility 计算波动率
func calculateVolatility(klines []Kline) float64 {
	if len(klines) < 2 {
		return 0
	}

	// 计算平均真实波幅
	sum := 0.0
	for i := 1; i < len(klines); i++ {
		high := klines[i].High
		low := klines[i].Low
		prevClose := klines[i-1].Close

		tr1 := high - low
		tr2 := math.Abs(high - prevClose)
		tr3 := math.Abs(low - prevClose)

		tr := math.Max(tr1, math.Max(tr2, tr3))
		sum += tr / prevClose * 100 // 转换为百分比
	}

	return sum / float64(len(klines)-1)
}

// parseFloat 解析float值
func parseFloat(v interface{}) (float64, error) {
	switch val := v.(type) {
	case string:
		return strconv.ParseFloat(val, 64)
	case float64:
		return val, nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	default:
		return 0, fmt.Errorf("unsupported type: %T", v)
	}
}
