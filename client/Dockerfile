FROM node:20-alpine AS build
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
ARG VITE_API_URL
ENV VITE_API_URL=$VITE_API_URL
RUN npm run build

FROM nginx:alpine
# Copy the built assets into a /server-hub subfolder
COPY --from=build /app/dist /usr/share/nginx/html/server-hub
RUN echo 'server { \
    listen 80; \
    location /server-hub/ { \
        alias /usr/share/nginx/html/server-hub/; \
        index index.html; \
        try_files $uri $uri/ /server-hub/index.html; \
    } \
    # Redirect root to /server-hub/ \
    location = / { \
        return 301 /server-hub/; \
    } \
}' > /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
