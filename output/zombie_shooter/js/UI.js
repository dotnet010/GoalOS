class UI {
  constructor() {
    this.el = {
      score: document.getElementById('scoreValue'),
      wave: document.getElementById('waveValue'),
      zombies: document.getElementById('zombiesValue'),
      healthBar: document.getElementById('healthBar'),
      healthText: document.getElementById('healthText'),
      ammo: document.getElementById('ammoValue'),
      announce: document.getElementById('waveAnnouncement'),
      gameOver: document.getElementById('gameOverScreen'),
      finalScore: document.getElementById('finalScore'),
      finalWave: document.getElementById('finalWave'),
      grenade: document.getElementById('skillGrenade'),
      timeSlow: document.getElementById('skillTimeSlow'),
      weapons: [document.getElementById('weapon1'), document.getElementById('weapon2'), document.getElementById('weapon3')]
    };
  }
  update() {
    this.el.score.textContent = Game.score;
    this.el.wave.textContent = Game.wave;
    this.el.zombies.textContent = Game.waveSystem ? Game.waveSystem.getAliveCount() : 0;
    const hp = (Game.player.health / Game.player.maxHealth) * 100;
    this.el.healthBar.style.width = hp + '%';
    this.el.healthText.textContent = Math.ceil(Game.player.health);
    if (Game.skillSystem) {
      const gc = Game.skillSystem.grenadeCD;
      if (gc > 0) { this.el.grenade.classList.remove('ready'); this.el.grenade.style.opacity = '0.5'; this.el.grenade.textContent = 'Q: Grenade (' + gc.toFixed(1) + 's)'; }
      else { this.el.grenade.classList.add('ready'); this.el.grenade.style.opacity = '1'; this.el.grenade.textContent = 'Q: Grenade [READY]'; }
      const tc = Game.skillSystem.timeSlowCD;
      if (tc > 0) { this.el.timeSlow.classList.remove('ready'); this.el.timeSlow.style.opacity = '0.5'; this.el.timeSlow.textContent = 'E: Time Slow (' + tc.toFixed(1) + 's)'; }
      else { this.el.timeSlow.classList.add('ready'); this.el.timeSlow.style.opacity = '1'; this.el.timeSlow.textContent = 'E: Time Slow [READY]'; }
    }
    if (Game.isGameOver) {
      this.el.gameOver.style.display = 'block';
      this.el.finalScore.textContent = Game.score;
      this.el.finalWave.textContent = Game.wave;
    }
  }
  updateWeaponDisplay() {
    const names = ['pistol', 'shotgun', 'rifle'];
    this.el.weapons.forEach((el, i) => { el.classList.toggle('active', Game.weaponSystem.current === names[i]); });
  }
  updateAmmoDisplay() {
    const w = Game.weaponSystem;
    if (w.ammo[w.current] === Infinity) this.el.ammo.textContent = '\u221e';
    else if (w.reloading) this.el.ammo.textContent = 'RELOADING...';
    else this.el.ammo.textContent = w.ammo[w.current] + ' / ' + WeaponConfig[w.current].maxAmmo;
  }
  showWaveAnnouncement(text) {
    this.el.announce.textContent = text;
    this.el.announce.classList.add('show');
    setTimeout(() => this.el.announce.classList.remove('show'), 2000);
  }
}