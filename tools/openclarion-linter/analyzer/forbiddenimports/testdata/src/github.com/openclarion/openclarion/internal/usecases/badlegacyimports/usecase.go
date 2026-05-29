package badlegacyimports

import (
	_ "github.com/gin-gonic/gin"               // want "forbidden module import"
	_ "github.com/go-redis/redis"              // want "forbidden module import"
	_ "github.com/gofiber/fiber"               // want "forbidden module import"
	_ "github.com/gomodule/redigo"             // want "forbidden module import"
	_ "github.com/labstack/echo"               // want "forbidden module import"
	_ "github.com/milvus-io/milvus-sdk-go"     // want "forbidden module import"
	_ "github.com/qdrant/go-client"            // want "forbidden module import"
	_ "github.com/redis/go-redis"              // want "forbidden module import"
	_ "github.com/weaviate/weaviate-go-client" // want "forbidden module import"
	_ "go.mongodb.org/mongo-driver"            // want "forbidden module import"
)
