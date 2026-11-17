# Stage 1: Build React + Vite SPA
FROM node:22-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci
COPY . .
ENV VITE_API_BASE_URL=""
ENV VITE_WS_URL="ws://localhost:5173/ws/telemetry"
RUN npm run build

# Stage 2: Serve via Nginx with API & WebSocket proxy
FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html

EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
