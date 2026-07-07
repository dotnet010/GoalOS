// UIManager - HUD, menus, screens
const UIManager = {
  el: {},

  init() {
    const g = id => document.getElementById(id);
    this.el = {
      healthBar: g('health-bar'), healthText: g('health-text'),
      score: g('score'), wave: g('wave'),
      weaponName: g('weapon-name'),
      grenadeCD: g('grenade-cd'), grenadeCount: g('grenade-count'),
      timeSlowCD: g('timeslow-cd'), timeSlowActive: g('timeslow-active'),
      startScreen: g('start-screen'), gameOverScreen: g('gameover-screen'),
      finalScore: g('final-score'), finalWave: g('final-wave'),
      waveAnnounce: g('wave-announce'),
      slots: [g('slot-0'), g('slot-1'), g('slot-2')]
    };
  },

  update() {
    const hp = PlayerController.health;
    const pct = (hp / PlayerController.maxHealth) * 100;
    this.el.healthBar.style.width = pct + '%';
    this.el.healthBar.style.backgroundColor = pct > 50 ? '#4caf50' : pct > 25 ? '#ff9800' : '#f44336';
    this.el.healthText.textContent = Math.ceil(hp) + ' / ' + PlayerController.maxHealth;
    this.el.score.textContent = GameEngine.score.toLocaleString();
    this.el.wave.textContent = WaveManager.currentWave;
    this.el.weaponName.textContent = WeaponSystem.weapons[WeaponSystem.currentWeapon].name;

    for (let i = 0; i < 3; i++) {
      if (this.el.slots[i]) this.el.slots[i].classList.toggle('active', i === WeaponSystem.currentWeapon);
    }

    const gCD = SkillSystem.grenadeCD;
    this.el.grenadeCD.style.width = (gCD > 0 ? (1 - gCD / SkillSystem.grenadeCDMax) * 100 : 100) + '%';
    this.el.grenadeCount.textContent = SkillSystem.grenadeCount;

    const tsCD = SkillSystem.timeSlowCD;
    this.el.timeSlowCD.style.width = (tsCD > 0 ? (1 - tsCD / SkillSystem.timeSlowCDMax) * 100 : 100) + '%';
    this.el.timeSlowActive.style.display = SkillSystem.timeSlowDuration > 0 ? 'block' : 'none';
  },

  showStartScreen() {
    this.el.startScreen.style.display = 'flex';
    this.el.gameOverScreen.style.display = 'none';
  },

  hideStartScreen() { this.el.startScreen.style.display = 'none'; },

  showGameOver() {
    this.el.gameOverScreen.style.display = 'flex';
    this.el.finalScore.textContent = GameEngine.score.toLocaleString();
    this.el.finalWave.textContent = WaveManager.currentWave;
  },

  announceWave(n) {
    const el = this.el.waveAnnounce;
    el.textContent = 'WAVE ' + n;
    el.style.display = 'block';
    el.style.animation = 'none';
    void el.offsetHeight;
    el.style.animation = 'waveAnnounce 2s ease-out forwards';
    setTimeout(() => { el.style.display = 'none'; }, 2000);
  }
};