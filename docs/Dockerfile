FROM nginx:1.13.9-alpine

ARG VERSION
ARG PRODUCT_SCOPE
ARG FROM_DIR

# Replace existing NGINX configuration
RUN rm -rf /etc/nginx/conf.d
COPY conf /etc/nginx

# Copy over both regular and no_auth bundles
COPY $FROM_DIR /usr/share/nginx/html/no_auth/$PRODUCT_SCOPE/$VERSION

EXPOSE 8080
CMD ["nginx", "-g", "daemon off;"]

