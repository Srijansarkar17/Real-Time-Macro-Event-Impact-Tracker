# Real-Time-Macro-Event-Impact-Tracker

## Architecture
![Architecture](images/architecture.png)

## Step1. Defining Assets to Track
We track 4 asset classes:
- Equities(SPY (S&P 500 ETF))
- FX (DXY or EURUSD)
- Rates (US 2Y Treasury yield, US 10Y Treasury yield)
- Volatility(VIX)


## What is CPI data? (Consumer Price Index data.)
It measures the average change over time in prices paid by consumers for goods and services.
In simple terms : CPI tells us how expensive life is becoming

CPI tracks price changes in common categories like:


Lets take an example of a basket
![CPI Formula](images/cpiformula.png)

- If CPI increases → Inflation
- If CPI decreases → Deflation

## Explaination of FetchMarketData() in market/fetch.go

- Fetches market data, For multiple assets, At the same time (concurrently), And waits until all are done

## Difference between FredSeries data and FredReleaseDate data?
- FredSeries = Economic Time Series Data(The historical inflation values.)
- FredRelease = These are the days when CPI was released to the public.