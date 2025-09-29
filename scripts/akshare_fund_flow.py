#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
AKShare资金流向数据获取脚本
"""
import json
import sys
import akshare as ak
from datetime import datetime

def get_fund_flow_data(stock_code):
    """
    获取个股资金流向数据
    
    Args:
        stock_code: 股票代码，格式如 "600000"
        
    Returns:
        dict: 资金流向数据
    """
    try:
        # 确定市场
        if stock_code.startswith('60') or stock_code.startswith('68') or stock_code.startswith('51'):
            market = 'sh'
        elif stock_code.startswith('00') or stock_code.startswith('30'):
            market = 'sz'
        else:
            return None
            
        # 获取资金流向数据
        df = ak.stock_individual_fund_flow(stock=stock_code, market=market)
        
        if df.empty:
            return None
            
        # 获取最新一天的数据
        latest_data = df.iloc[-1]
        
        # 构造返回数据
        fund_flow = {
            "main_net_inflow": float(latest_data.get('主力净流入-净额', 0)),
            "super_large_net_inflow": float(latest_data.get('超大单净流入-净额', 0)),
            "large_net_inflow": float(latest_data.get('大单净流入-净额', 0)),
            "medium_net_inflow": float(latest_data.get('中单净流入-净额', 0)),
            "small_net_inflow": float(latest_data.get('小单净流入-净额', 0)),
            "net_inflow_ratio": float(latest_data.get('主力净流入-净占比', 0)),
            "active_buy_amount": float(latest_data.get('主力买入成交额', 0)),
            "active_sell_amount": float(latest_data.get('主力卖出成交额', 0))
        }
        
        return fund_flow
        
    except Exception as e:
        print(f"Error getting fund flow data for {stock_code}: {e}", file=sys.stderr)
        return None

def main():
    """主函数"""
    if len(sys.argv) != 2:
        print("Usage: python akshare_fund_flow.py <stock_code>")
        sys.exit(1)
        
    stock_code_input = sys.argv[1]
    
    # 提取纯数字股票代码 (去掉SH/SZ前缀)
    if stock_code_input.startswith(('SH', 'SZ')):
        stock_code = stock_code_input[2:]
    else:
        stock_code = stock_code_input
        
    fund_flow_data = get_fund_flow_data(stock_code)
    
    if fund_flow_data:
        print(json.dumps(fund_flow_data, ensure_ascii=False))
    else:
        print(json.dumps({
            "main_net_inflow": 0,
            "super_large_net_inflow": 0,
            "large_net_inflow": 0,
            "medium_net_inflow": 0,
            "small_net_inflow": 0,
            "net_inflow_ratio": 0,
            "active_buy_amount": 0,
            "active_sell_amount": 0
        }))

if __name__ == "__main__":
    main()