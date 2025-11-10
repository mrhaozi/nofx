import type { AIDecisionData } from '../../types';

interface AIDecisionDisplayProps {
  data: AIDecisionData;
}

export default function AIDecisionDisplay({ data }: AIDecisionDisplayProps) {
  const getDecisionColor = (decision: string) => {
    switch (decision.toLowerCase()) {
      case 'buy':
      case 'open_long':
        return 'text-green-600 bg-green-100';
      case 'sell':
      case 'open_short':
        return 'text-red-600 bg-red-100';
      case 'hold':
      case 'wait':
      case 'close_long':
      case 'close_short':
        return 'text-gray-600 bg-gray-100';
      default:
        return 'text-blue-600 bg-blue-100';
    }
  };

  const getConfidenceColor = (confidence: number) => {
    if (confidence >= 80) return 'text-green-600';
    if (confidence >= 60) return 'text-yellow-600';
    return 'text-red-600';
  };

  return (
    <div className="bg-white shadow rounded-lg p-6">
      <h2 className="text-xl font-semibold mb-4 text-gray-800">AI决策结果</h2>

      <div className="space-y-6">
        {/* 主要决策信息 */}
        <div className="flex items-center gap-6 p-4 bg-gray-50 rounded-lg">
          <div className="flex items-center gap-3">
            <span className="text-gray-600 font-medium">决策:</span>
            <span className={`px-3 py-1 rounded-full font-medium ${getDecisionColor(data.decision)}`}>
              {data.decision.toUpperCase()}
            </span>
          </div>

          <div className="flex items-center gap-3">
            <span className="text-gray-600 font-medium">置信度:</span>
            <span className={`font-medium ${getConfidenceColor(data.confidence)}`}>
              {(data.confidence * 100).toFixed(1)}%
            </span>
          </div>

          <div className="flex items-center gap-3">
            <span className="text-gray-600 font-medium">响应时间:</span>
            <span className="font-medium text-gray-800">{data.responseTime}ms</span>
          </div>
        </div>

        {/* 交易参数 */}
        {data.parameters && Object.keys(data.parameters).length > 0 && (
          <div>
            <h3 className="text-lg font-medium text-gray-800 mb-3">交易参数</h3>
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {data.parameters.leverage && (
                <div className="bg-blue-50 p-3 rounded-lg border border-blue-200">
                  <div className="text-sm text-blue-600 font-medium">杠杆倍数</div>
                  <div className="text-lg font-semibold text-blue-800">{data.parameters.leverage}x</div>
                </div>
              )}
              {data.parameters.positionSizeUSD && (
                <div className="bg-green-50 p-3 rounded-lg border border-green-200">
                  <div className="text-sm text-green-600 font-medium">仓位大小</div>
                  <div className="text-lg font-semibold text-green-800">${data.parameters.positionSizeUSD.toFixed(2)}</div>
                </div>
              )}
              {data.parameters.stopLoss && (
                <div className="bg-red-50 p-3 rounded-lg border border-red-200">
                  <div className="text-sm text-red-600 font-medium">止损价格</div>
                  <div className="text-lg font-semibold text-red-800">${data.parameters.stopLoss.toFixed(2)}</div>
                </div>
              )}
              {data.parameters.takeProfit && (
                <div className="bg-green-50 p-3 rounded-lg border border-green-200">
                  <div className="text-sm text-green-600 font-medium">止盈价格</div>
                  <div className="text-lg font-semibold text-green-800">${data.parameters.takeProfit.toFixed(2)}</div>
                </div>
              )}
              {data.parameters.riskUSD && (
                <div className="bg-yellow-50 p-3 rounded-lg border border-yellow-200">
                  <div className="text-sm text-yellow-600 font-medium">风险金额</div>
                  <div className="text-lg font-semibold text-yellow-800">${data.parameters.riskUSD.toFixed(2)}</div>
                </div>
              )}
            </div>
          </div>
        )}

        {/* 决策理由 */}
        <div>
          <h3 className="text-lg font-medium text-gray-800 mb-3">决策理由</h3>
          <div className="bg-blue-50 p-4 rounded-lg border border-blue-200">
            <p className="text-blue-900 leading-relaxed">{data.reasoning}</p>
          </div>
        </div>

        {/* 思维链分析 */}
        {data.cotTrace && (
          <div>
            <h3 className="text-lg font-medium text-gray-800 mb-3">思维链分析</h3>
            <div className="bg-indigo-50 p-4 rounded-lg border border-indigo-200">
              <pre className="text-indigo-900 whitespace-pre-wrap font-sans leading-relaxed">{data.cotTrace}</pre>
            </div>
          </div>
        )}

        {/* 原始响应 */}
        <div>
          <h3 className="text-lg font-medium text-gray-800 mb-3">AI原始响应</h3>
          <div className="bg-gray-100 p-4 rounded-lg border">
            <pre className="text-xs text-gray-600 whitespace-pre-wrap font-mono leading-relaxed max-h-48 overflow-auto">{data.aiResponse}</pre>
          </div>
        </div>

        {/* 底部信息 */}
        <div className="flex justify-between items-center pt-4 border-t text-sm text-gray-500">
          <div>
            <span>生成时间: </span>
            <span className="font-medium">{new Date(data.timestamp).toLocaleString()}</span>
          </div>
          <div>
            <span>交易对: </span>
            <span className="font-medium">{data.symbol}</span>
          </div>
        </div>
      </div>
    </div>
  );
}