import os
import sys
import redis


def main() -> int:
    """
    Checks worker access to the shared Redis service.

    The health check deliberately avoids importing ML services so Docker health
    probes never load Qwen, Faster-Whisper, FastEmbed, or FAISS artifacts.

    Returns:
        int: Zero when Redis is reachable, otherwise one.
    """
    redis_url = os.getenv("REDIS_URL", "redis://redis:6379").strip()

    if not redis_url.startswith(("redis://", "rediss://")):
        redis_url = f"redis://{redis_url}"

    try:
        client = redis.from_url(
            redis_url,
            socket_connect_timeout=2,
            socket_timeout=2,
        )
        return 0 if client.ping() else 1
    except redis.RedisError:
        return 1


if __name__ == "__main__":
    sys.exit(main())