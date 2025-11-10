import { useState } from 'react';
import type { UserPromptData } from '../../types';

interface UserPromptDisplayProps {
  data: UserPromptData;
  onGetAIDecision: () => void;
  isLoading: boolean;
}

export default function UserPromptDisplay({ data, onGetAIDecision, isLoading }: UserPromptDisplayProps) {
  const [isExpanded, setIsExpanded] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(data.userPrompt);
      // 这里可以添加一个toast提示
      console.log('UserPrompt已复制到剪贴板');
    } catch (err) {
      console.error('复制失败:', err);
    }
  };

  return (
    <div className="bg-white shadow rounded-lg p-6">
      {/* 标题栏 */}
      <div className="flex justify-between items-center mb-4">
        <h2 className="text-xl font-semibold text-gray-800">UserPrompt内容</h2>
        <div className="flex gap-2">
          <button
            onClick={handleCopy}
            className="px-3 py-1 text-sm bg-gray-100 hover:bg-gray-200 text-gray-700 rounded-md transition-colors flex items-center gap-1"
            title="复制UserPrompt"
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
            </svg>
            复制
          </button>
          <button
            onClick={() => setIsExpanded(!isExpanded)}
            className="px-3 py-1 text-sm bg-gray-100 hover:bg-gray-200 text-gray-700 rounded-md transition-colors"
          >
            {isExpanded ? '收起' : '展开'}
          </button>
        </div>
      </div>

      {/* 市场数据概览 */}
      <div className="mb-6">
        <h3 className="text-sm font-medium text-gray-700 mb-3">市场数据概览</h3>
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          <div className="bg-gray-50 p-3 rounded-lg">
            <div className="text-xs text-gray-500">当前价格</div>
            <div className="text-lg font-semibold text-gray-800">${data.marketData.currentPrice.toFixed(2)}</div>
          </div>
          <div className="bg-gray-50 p-3 rounded-lg">
            <div className="text-xs text-gray-500">1小时涨跌</div>
            <div className={`text-lg font-semibold ${
              data.marketData.priceChange1h >= 0 ? 'text-green-600' : 'text-red-600'
            }`}>
              {data.marketData.priceChange1h >= 0 ? '+' : ''}{data.marketData.priceChange1h.toFixed(2)}%
            </div>
          </div>
          <div className="bg-gray-50 p-3 rounded-lg">
            <div className="text-xs text-gray-500">4小时涨跌</div>
            <div className={`text-lg font-semibold ${
              data.marketData.priceChange4h >= 0 ? 'text-green-600' : 'text-red-600'
            }`}>
              {data.marketData.priceChange4h >= 0 ? '+' : ''}{data.marketData.priceChange4h.toFixed(2)}%
            </div>
          </div>
          <div className="bg-gray-50 p-3 rounded-lg">
            <div className="text-xs text-gray-500">成交量</div>
            <div className="text-lg font-semibold text-gray-800">{data.marketData.volume.toFixed(0)}</div>
          </div>
        </div>
      </div>

      {/* UserPrompt内容 */}
      <div className="mb-6">
        <div className="flex justify-between items-center mb-3">
          <h3 className="text-sm font-medium text-gray-700">UserPrompt详细内容</h3>
          <span className="text-xs text-gray-500">
            生成时间: {new Date(data.timestamp).toLocaleString()}
          </span>
        </div>

        <div className="relative">
          <pre
            className={`bg-gray-50 p-4 rounded-lg overflow-auto text-sm font-mono text-gray-800 border ${
              isExpanded ? 'max-h-96' : 'max-h-32'
            } transition-all`}
          >
            {data.userPrompt}
          </pre>

          {!isExpanded && (
            <div className="absolute inset-x-0 bottom-0 h-8 bg-gradient-to-t from-gray-50 to-transparent pointer-events-none" />
          )}
        </div>
      </div>

      {/* 操作按钮 */}
      <div className="flex justify-between items-center pt-4 border-t">
        <div className="text-sm text-gray-500">
          <span>交易对: </span>
          <span className="font-medium text-gray-700">{data.symbol}</span>
        </div>

        <button
          onClick={onGetAIDecision}
          disabled={isLoading}
          className="px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700 disabled:opacity-50 disabled:cursor-not-allowed font-medium transition-colors flex items-center gap-2"
        >
          {isLoading ? (
            <>
              <svg className="animate-spin h-4 w-4" viewBox="0 0 24 24">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" fill="none"/>
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"/>
              </svg>
              获取中...
            </>
          ) : (
            <>
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z" />
              </svg>
              获取AI决策
            </>
          )}
        </button>
      </div>
    </div>
  );
}