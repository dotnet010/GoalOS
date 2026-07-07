// WaveManager - Wave system with difficulty scaling
const WaveManager = {
  currentWave: 0,
  zombiesToSpawn: 0,
  spawnTimer: 0,
  spawnInterval: 2.0,
  interWaveTimer: 0,
  inBreak: false,
  breakDuration: 5,
  waveActive: false,

  init() {
    this.currentWave = 0;
    this.zombiesToSpawn = 0;
    this.spawnTimer = 0;
    this.interWaveTimer = 0;
    this.inBreak = false;
    this.waveActive = false;
  },

  startNextWave() {
    this.currentWave++;
    this.zombiesToSpawn = 5 + this.currentWave * 3;
    this.spawnInterval = Math.max(0.4, 2.0 - this.currentWave * 0.1);
    this.inBreak = false;
    this.waveActive = true;
    this.spawnTimer = 0;
  },

  getZombieTypeForWave() {
    const w = this.currentWave;
    const r = Math.random();
    if (w <= 2) return 'normal';
    if (w <= 5) return r < 0.7 ? 'normal' : 'fast';
    if (r < 0.5) return 'normal';
    if (r < 0.8) return 'fast';
    return 'large';
  },

  update(delta) {
    if (this.inBreak) {
      this.interWaveTimer -= delta;
      if (this.interWaveTimer <= 0) this.startNextWave();
      return;
    }
    if (this.waveActive && this.zombiesToSpawn > 0) {
      this.spawnTimer -= delta;
      if (this.spawnTimer <= 0) {
        this.spawnTimer = this.spawnInterval;
        ZombieManager.spawnAtEdge(this.getZombieTypeForWave());
        this.zombiesToSpawn--;
      }
    }
    if (this.waveActive && this.zombiesToSpawn <= 0 && ZombieManager.zombies.length === 0) {
      this.waveActive = false;
      this.inBreak = true;
      this.interWaveTimer = this.breakDuration;
      GameEngine.score += this.currentWave * 50;
    }
  },

  reset() { this.init(); }
};