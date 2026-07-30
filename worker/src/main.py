from broker.consumer import start_worker
from utils.logger import get_logger

logger = get_logger("main")


if __name__ == "__main__":
    logger.info("Starting Lexos Worker service...")
    start_worker()