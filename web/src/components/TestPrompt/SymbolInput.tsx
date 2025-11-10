import { useState } from 'react';

interface SymbolInputProps {
  symbol: string;
  onSymbolChange: (symbol: string) => void;
  onGetUserPrompt: () => void;
  isLoading: boolean;
  disabled?: boolean;
}

export default function SymbolInput({
  symbol,
  onSymbolChange,
  onGetUserPrompt,
  isLoading,
  disabled = false
}: SymbolInputProps) {
  const [inputValue, setInputValue] = useState(symbol);

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value.toUpperCase().replace(/[^A-Z0-9]/g, '');
    setInputValue(value);
    onSymbolChange(value);
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!inputValue.trim()) return;
    onGetUserPrompt();
  };

  const handleQuickSelect = (quickSymbol: string) => {
    setInputValue(quickSymbol);
    onSymbolChange(quickSymbol);
  };

  const commonSymbols = ['BTCUSDT', 'ETHUSDT', 'SOLUSDT', 'BNBUSDT', 'XRPUSDT'];

  return (
    <div className="bg-white shadow rounded-lg p-6">
      <h2 className="text-xl font-semibold mb-4 text-gray-800">输入交易对</h2>

      <form onSubmit={handleSubmit} className="space-y-4">
        <div className="flex gap-4">
          <div className="flex-1">
            <input
              type="text"
              value={inputValue}
              onChange={handleInputChange}
              placeholder="例如: BTCUSDT"
              className="w-full px-4 py-3 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent transition-all"
              disabled={isLoading || disabled}
            />
          </div>

          <button
            type="submit"
            disabled={isLoading || disabled || !inputValue.trim()}
            className="px-6 py-3 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed font-medium transition-colors flex items-center gap-2"
          >
            {isLoading ? (
              <>
                <svg className="animate-spin h-4 w-4" viewBox="0 0 24 24">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" fill="none"/>
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"/>
                </svg>
                生成中...
              </>
            ) : (
              '生成UserPrompt'
            )}
          </button>
        </div>

        <div className="text-sm text-gray-500">
          <p>输入交易对符号，如BTCUSDT、ETHUSDT等</p>
        </div>
      </form>

      {/* 常用交易对快速选择 */}
      <div className="mt-4">
        <p className="text-sm text-gray-600 mb-2">常用交易对：</p>
        <div className="flex flex-wrap gap-2">
          {commonSymbols.map((quickSymbol) => (
            <button
              key={quickSymbol}
              type="button"
              onClick={() => handleQuickSelect(quickSymbol)}
              disabled={isLoading || disabled}
              className={`px-3 py-1 text-xs rounded-full border transition-colors ${
                inputValue === quickSymbol
                  ? 'bg-blue-100 border-blue-300 text-blue-700'
                  : 'bg-gray-100 border-gray-300 text-gray-700 hover:bg-gray-200'
              } disabled:opacity-50`}
            >
              {quickSymbol}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}