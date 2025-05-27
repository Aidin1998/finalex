# Market Maker Bot

## Overview
The Market Maker Bot is a sophisticated trading application designed to facilitate market making across multiple cryptocurrency exchanges. It utilizes advanced strategies and real-time data processing to optimize trading performance while managing risks effectively.

## Project Structure
The project is organized into several modules, each responsible for specific functionalities:

- **API Connectivity Module**: Handles WebSocket and REST connections to multiple exchanges.
  - `api/connectivity.go`
  - `api/exchanges.go`
  - `api/rate_limit.go`

- **Order Book Management**: Implements an in-memory radix tree for efficient order management and real-time data processing.
  - `orderbook/radix_tree.go`
  - `orderbook/aggregation.go`

- **Market Making Strategy Engine**: Contains the core logic for executing market making strategies.
  - `strategy/engine.go`
  - `strategy/llm_integration.go`

- **Risk Management Module**: Monitors and manages inventory and volatility risks.
  - `risk/inventory.go`
  - `risk/volatility.go`

- **Order Execution Module**: Manages the lifecycle of orders and adjustments.
  - `execution/order_lifecycle.go`
  - `execution/adjustments.go`

- **Monitoring & Logging Module**: Implements real-time alerts and structured logging for performance tracking.
  - `monitoring/alerts.go`
  - `monitoring/logging.go`

- **LLM Integration**: Prepares for future integration with a Python-hosted LLM via API.
  - `llm/api_client.go`

## Setup Instructions
1. **Clone the Repository**
   ```bash
   git clone https://github.com/microsoft/vscode-remote-try-go.git
   cd market-maker-bot
   ```

2. **Install Dependencies**
   Ensure you have Go installed. Run the following command to download the necessary dependencies:
   ```bash
   go mod tidy
   ```

3. **Build the Docker Image**
   To build the Docker image, run:
   ```bash
   docker build -t market-maker-bot .
   ```

4. **Run the Application**
   You can run the application using Docker:
   ```bash
   docker run -p 8080:8080 market-maker-bot
   ```

## Usage
Once the application is running, it will connect to the specified exchanges and begin executing market making strategies based on the implemented algorithms.

## Contribution Guidelines
Contributions are welcome! Please fork the repository and submit a pull request with your changes. Ensure that your code adheres to the project's coding standards and includes appropriate tests.

## License
This project is licensed under the MIT License. See the LICENSE file for more details.