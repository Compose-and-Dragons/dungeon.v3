# Dungeon.v3
> - With Genkit, Docker Model Runner, Docker Agentic Compose and Docker MCP Gateway.
> - **What is new**: use the Genkit **flows** to implement AI Agents


## Getting Started

```bash
docker compose up --build -d
docker compose logs mcp-gateway

docker compose attach dungeon-master
# then start: ./dungeon-master
# or
docker compose exec -it dungeon-master ./dungeon-master
```