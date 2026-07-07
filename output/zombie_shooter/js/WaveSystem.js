class WaveSystem {
  constructor() {
    this.wave = 0; this.zombiesToSpawn = 0; this.spawnTimer = 0;
    this.spawnInterval = 1.5; this.interWaveTimer = 3; this.inWave = false;
  }
  startWave() {
    this.wave++;
    this.zombiesToSpawn = 5 + this.wave * 3;
    this.spawnInterval = Math.max(0.3, 1.5 - this.wave * 0.05);
    this.inWave = true; this.spawnTimer = 0;
    Game.wave = this.wave;
    Game.ui.showWaveAnnouncement('WAVE ' + this.wave);
  }
  spawnZombie() {
    const types = ['normal'];
    if (this.wave >= 2) types.push('fast');
    if (this.wave >= 3) types.push('normal');
    if (this.wave >= 4) types.push('large');
    if (this.wave >= 6) types.push('large', 'fast');
    const type = types[Math.floor(Math.random() * types.length)];
    const a = Math.random() * Math.PI * 2;
    const d = Game.WORLD_SIZE - 3;
    Game.zombies.push(new Zombie(type, new THREE.Vector3(Math.cos(a) * d, 0, Math.sin(a) * d)));
  }
  update(dt) {
    if (!this.inWave) {
      this.interWaveTimer -= dt;
      if (this.interWaveTimer <= 0) this.startWave();
      return;
    }
    if (this.zombiesToSpawn > 0) {
      this.spawnTimer -= dt;
      if (this.spawnTimer <= 0) { this.spawnZombie(); this.zombiesToSpawn--; this.spawnTimer = this.spawnInterval; }
    }
    if (this.zombiesToSpawn === 0 && this.getAliveCount() === 0) {
      this.inWave = false; this.interWaveTimer = 5;
      Game.ui.showWaveAnnouncement('WAVE ' + this.wave + ' CLEARED!');
    }
  }
  getAliveCount() { return Game.zombies.filter(z => z.alive).length; }
}