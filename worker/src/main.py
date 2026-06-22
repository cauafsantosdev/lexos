import time

def main():
    print("Lexos Worker is running!")
    print("Waiting for Redis connection...")
    
    # Infinite loop to keep the container running
    while True:
        time.sleep(10)

if __name__ == "__main__":
    main()