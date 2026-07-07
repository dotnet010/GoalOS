Game.init();
Game.player = new Player();
Game.particleSystem = new ParticleSystem();
Game.weaponSystem = new WeaponSystem();
Game.skillSystem = new SkillSystem();
Game.waveSystem = new WaveSystem();
Game.ui = new UI();
Game.ui.updateWeaponDisplay();
Game.ui.updateAmmoDisplay();
Game.isRunning = true;

function animate() {
  requestAnimationFrame(animate);
  const dt = Math.min(Game.clock.getDelta(), 0.05);
  if (Game.isRunning && !Game.isGameOver) {
    Game.player.update(dt);
    Game.weaponSystem.update(dt);
    Game.skillSystem.update(dt);
    Game.waveSystem.update(dt);
    for (let i = Game.zombies.length - 1; i >= 0; i--) {
      Game.zombies[i].update(dt);
      if (!Game.zombies[i].alive) Game.zombies.splice(i, 1);
    }
    for (let i = Game.projectiles.length - 1; i >= 0; i--) {
      if (!Game.projectiles[i].update(dt)) { Game.projectiles[i].destroy(); Game.projectiles.splice(i, 1); }
    }
    Game.particleSystem.update(dt);
    const tx = Game.player.mesh.position.x, tz = Game.player.mesh.position.z;
    Game.camera.position.x += (tx - Game.camera.position.x) * 0.1;
    Game.camera.position.y = 35;
    Game.camera.position.z += (tz + 25 - Game.camera.position.z) * 0.1;
    Game.camera.lookAt(tx, 0, tz);
    Game.updateMouseWorld();
  }
  Game.ui.update();
  Game.renderer.render(Game.scene, Game.camera);
}
animate();