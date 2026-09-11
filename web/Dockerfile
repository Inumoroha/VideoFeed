# syntax=docker/dockerfile:1
# 在仓库根目录执行：docker build -t videofeed-web ./web

FROM node:22-alpine AS builder

WORKDIR /src

COPY package*.json ./
RUN npm ci

COPY . .
RUN npm run build

FROM nginx:1.27-alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /src/dist /usr/share/nginx/html

EXPOSE 80
