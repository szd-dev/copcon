#!/bin/bash
# Initialize Qdrant collection for Agent Memory

QDRANT_HOST="${QDRANT_HOST:-localhost}"
QDRANT_PORT="${QDRANT_PORT:-6333}"
COLLECTION_NAME="agent_memory"
VECTOR_SIZE=1536

echo "Initializing Qdrant collection: $COLLECTION_NAME"
echo "Qdrant endpoint: http://$QDRANT_HOST:$QDRANT_PORT"

# Check if Qdrant is running
echo "Checking Qdrant health..."
curl -s "http://$QDRANT_HOST:$QDRANT_PORT/health" || {
    echo "Error: Qdrant is not running at http://$QDRANT_HOST:$QDRANT_PORT"
    exit 1
}

# Check if collection exists
COLLECTION_EXISTS=$(curl -s "http://$QDRANT_HOST:$QDRANT_PORT/collections/$COLLECTION_NAME/exist" | grep -o '"result":true')

if [ "$COLLECTION_EXISTS" ]; then
    echo "Collection '$COLLECTION_NAME' already exists"
else
    echo "Creating collection '$COLLECTION_NAME'..."
    
    # Create collection with Cosine distance
    curl -X PUT "http://$QDRANT_HOST:$QDRANT_PORT/collections/$COLLECTION_NAME" \
        -H "Content-Type: application/json" \
        -d '{
            "vectors": {
                "size": '$VECTOR_SIZE',
                "distance": "Cosine"
            },
            "optimizers_config": {
                "indexing_threshold": 20000
            },
            "quantization_config": {
                "scalar": {
                    "type": "int8",
                    "quantile": 0.99,
                    "always_ram": true
                }
            }
        }'
    
    echo ""
    echo "Collection created successfully"
fi

# Create payload indexes for efficient filtering
echo "Creating payload indexes..."

# Index on session_id
curl -X PUT "http://$QDRANT_HOST:$QDRANT_PORT/collections/$COLLECTION_NAME/index" \
    -H "Content-Type: application/json" \
    -d '{"field_name": "session_id", "field_schema": "keyword"}'

# Index on memory_type
curl -X PUT "http://$QDRANT_HOST:$QDRANT_PORT/collections/$COLLECTION_NAME/index" \
    -H "Content-Type: application/json" \
    -d '{"field_name": "memory_type", "field_schema": "keyword"}'

# Index on timestamp
curl -X PUT "http://$QDRANT_HOST:$QDRANT_PORT/collections/$COLLECTION_NAME/index" \
    -H "Content-Type: application/json" \
    -d '{"field_name": "timestamp", "field_schema": "integer"}'

echo ""
echo "Qdrant initialization complete!"
echo "Collection: $COLLECTION_NAME"
echo "Vector size: $VECTOR_SIZE"
echo "Distance: Cosine"