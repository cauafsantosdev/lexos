import sys
import logging


def get_logger(name: str) -> logging.Logger:
    """
    Creates and returns a pre-configured logger with a standard timestamp, 
    level, and module name format. Ensures Docker captures logs instantly via sys.stdout.

    Args:
        name (str): The logical name of the logger (e.g., 'consumer', 'distiller').

    Returns:
        logging.Logger: The configured standard logger instance.
    """
    logger = logging.getLogger(name)
    
    # Prevent adding multiple handlers if the logger is requested multiple times
    if not logger.handlers:
        logger.setLevel(logging.INFO)
        
        # Ensures Docker captures the logs instantly
        handler = logging.StreamHandler(sys.stdout)
        
        # Format: [2026-08-03 10:00:35] | INFO | consumer | Message here
        formatter = logging.Formatter(
            '%(asctime)s | %(levelname)-s | %(name)s | %(message)s',
            datefmt='%Y-%m-%d %H:%M:%S'
        )
        
        handler.setFormatter(formatter)
        logger.addHandler(handler)
        
    return logger