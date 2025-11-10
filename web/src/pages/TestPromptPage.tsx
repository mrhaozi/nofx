import { useState, useEffect } from 'react';
import type { UserPromptData, AIDecisionData, TraderInfo } from '../types';
import { api } from '../lib/api';
import SymbolInput from '../components/TestPrompt/SymbolInput';
import UserPromptDisplay from '../components/TestPrompt/UserPromptDisplay';
import AIDecisionDisplay from '../components/TestPrompt/AIDecisionDisplay';

export default function TestPromptPage() {
  const [symbol, setSymbol] = useState('');
  const [isLoadingUserPrompt, setIsLoadingUserPrompt] = useState(false);
  const [isLoadingAIDecision, setIsLoadingAIDecision] = useState(false);
  const [userPrompt, setUserPrompt] = useState<UserPromptData | null>(null);
  const [aiDecision, setAiDecision] = useState<AIDecisionData | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);
  const [traders, setTraders] = useState<TraderInfo[]>([]);
  const [selectedTraderId, setSelectedTraderId] = useState<string>('');

  // 加载交易员列表
  useEffect(() => {
    const loadTraders = async () => {
      try {
        const tradersData = await api.getTraders();
        setTraders(tradersData);
        // 如果有交易员，默认选择第一个
        if (tradersData.length > 0) {
          setSelectedTraderId(tradersData[0].trader_id);
        }
      } catch (err) {
        console.error('加载交易员列表失败:', err);
        setError('加载交易员列表失败');
      }
    };
    loadTraders();
  }, []);

  const handleGetUserPrompt = async () => {
    if (!symbol.trim()) {
      setError('请输入交易对');
      return;
    }

    if (!selectedTraderId) {
      setError('请选择交易员');
      return;
    }

    setIsLoadingUserPrompt(true);
    setError(null);
    setSuccessMessage(null);
    setAiDecision(null); // 清空之前的AI决策

    try {
      const data = await api.generateUserPrompt(symbol.trim(), selectedTraderId);
      setUserPrompt(data);
      setSuccessMessage('UserPrompt生成成功');
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : '获取UserPrompt失败';
      setError(errorMessage);
      console.error('生成UserPrompt失败:', err);
    } finally {
      setIsLoadingUserPrompt(false);
    }
  };

  const handleGetAIDecision = async () => {
    if (!userPrompt) return;

    if (!selectedTraderId) {
      setError('请选择交易员');
      return;
    }

    setIsLoadingAIDecision(true);
    setError(null);
    setSuccessMessage(null);

    try {
      const data = await api.testAIDecision(symbol, userPrompt.userPrompt, selectedTraderId);
      setAiDecision(data);
      setSuccessMessage('AI决策获取成功');
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : '获取AI决策失败';
      setError(errorMessage);
      console.error('获取AI决策失败:', err);
    } finally {
      setIsLoadingAIDecision(false);
    }
  };

  const clearResults = () => {
    setUserPrompt(null);
    setAiDecision(null);
    setError(null);
    setSuccessMessage(null);
  };

  return (
    <div className="container mx-auto p-6 max-w-6xl">
      {/* 页面标题 */}
      <div className="mb-8">
        <h1 className="text-3xl font-bold text-gray-900 mb-2">UserPrompt手动验证</h1>
        <p className="text-gray-600">
          输入交易对符号，获取对应的UserPrompt并测试AI决策逻辑
        </p>
      </div>

      {/* 消息提示 */}
      {(error || successMessage) && (
        <div className="mb-6">
          {error && (
            <div className="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded-lg mb-4">
              <div className="flex items-center gap-2">
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                {error}
              </div>
            </div>
          )}
          {successMessage && (
            <div className="bg-green-50 border border-green-200 text-green-700 px-4 py-3 rounded-lg mb-4">
              <div className="flex items-center gap-2">
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                {successMessage}
              </div>
            </div>
          )}
        </div>
      )}

      {/* 主要内容区域 */}
      <div className="space-y-6">
        {/* 交易员选择 */}
        <div className="bg-white rounded-lg p-6 shadow-sm border border-gray-200">
          <h3 className="text-lg font-semibold text-gray-900 mb-4">选择交易员</h3>
          <div className="flex items-center gap-4">
            <div className="flex-1">
              <label className="block text-sm font-medium text-gray-700 mb-2">交易员</label>
              <select
                value={selectedTraderId}
                onChange={(e) => setSelectedTraderId(e.target.value)}
                className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
              >
                <option value="">请选择交易员</option>
                {traders.map((trader) => (
                  <option key={trader.trader_id} value={trader.trader_id}>
                    {trader.trader_name} ({trader.ai_model})
                  </option>
                ))}
              </select>
            </div>
            <div className="flex-shrink-0 pt-6">
              <div className="text-sm text-gray-500">
                {selectedTraderId ? `已选择: ${traders.find(t => t.trader_id === selectedTraderId)?.trader_name}` : '请选择一个交易员'}
              </div>
            </div>
          </div>
        </div>

        {/* 步骤指示器 */}
        <div className="flex items-center justify-center mb-8">
          <div className="flex items-center space-x-4">
            <div className={`flex items-center space-x-2 ${
              userPrompt ? 'text-blue-600' : 'text-gray-400'
            }`}>
              <div className={`w-8 h-8 rounded-full flex items-center justify-center ${
                userPrompt ? 'bg-blue-600 text-white' : 'bg-gray-200 text-gray-500'
              }`}>
                1
              </div>
              <span className="font-medium">生成UserPrompt</span>
            </div>
            <div className={`w-16 h-1 ${
              userPrompt ? 'bg-blue-600' : 'bg-gray-200'
            }`} />
            <div className={`flex items-center space-x-2 ${
              aiDecision ? 'text-green-600' : 'text-gray-400'
            }`}>
              <div className={`w-8 h-8 rounded-full flex items-center justify-center ${
                aiDecision ? 'bg-green-600 text-white' : 'bg-gray-200 text-gray-500'
              }`}>
                2
              </div>
              <span className="font-medium">获取AI决策</span>
            </div>
          </div>
        </div>

        {/* Symbol输入组件 */}
        <SymbolInput
          symbol={symbol}
          onSymbolChange={setSymbol}
          onGetUserPrompt={handleGetUserPrompt}
          isLoading={isLoadingUserPrompt}
        />

        {/* UserPrompt显示 */}
        {userPrompt && (
          <UserPromptDisplay
            data={userPrompt}
            onGetAIDecision={handleGetAIDecision}
            isLoading={isLoadingAIDecision}
          />
        )}

        {/* AI决策显示 */}
        {aiDecision && (
          <AIDecisionDisplay
            data={aiDecision}
          />
        )}

        {/* 操作按钮 */}
        {(userPrompt || aiDecision) && (
          <div className="flex justify-center">
            <button
              onClick={clearResults}
              className="px-6 py-2 bg-gray-600 text-white rounded-lg hover:bg-gray-700 transition-colors font-medium"
            >
              清空结果
            </button>
          </div>
        )}
      </div>

      {/* 使用说明 */}
      <div className="mt-12 bg-blue-50 border border-blue-200 rounded-lg p-6">
        <h3 className="text-lg font-semibold text-blue-900 mb-3">使用说明</h3>
        <div className="text-blue-800 space-y-2">
          <p>• 输入交易对符号（如BTCUSDT）并点击"生成UserPrompt"</p>
          <p>• 系统会根据当前市场数据生成完整的UserPrompt内容</p>
          <p>• 点击"获取AI决策"按钮，将UserPrompt发送给AI模型进行分析</p>
          <p>• 查看AI返回的决策结果，包括交易建议、参数和理由</p>
          <p>• 此功能主要用于验证UserPrompt格式和测试AI决策逻辑</p>
        </div>
      </div>
    </div>
  );
}