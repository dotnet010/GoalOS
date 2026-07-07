// main.js - Entry point, game loop, module wiring
let mouseNDC = new THREE.Vector2(-1, -1);
let isFiring = false;
let lastWave = 0;

function initGame() {
  const container = document.getElementById('game-container');
  GameEngine.init(container);

  const ground = AssetFactory.createGround(100);
  GameEngine.scene.add(ground);

  const environment = AssetFactory.createEnvironment();
  GameEngine.scene.add(environment);
  PhysicsWorld.init(environment);

  PlayerController.init(GameEngine.scene);
  ZombieManager.init(GameEngine.scene);
  WeaponSystem.init(GameEngine.scene);
  SkillSystem.init(GameEngine.scene);
  ParticleSystem.init(GameEngine.scene);
  WaveManager.init();
  UIManager.init();

  window.addEventListener('mousemove', e => {
    mouseNDC.x = (e.clientX / window.innerWidth) * 2 - 1;
    mouseNDC.y = -(e.clientY / window.innerHeight) * 2 + 1;
  });
  window.addEventListener('mousedown', e => { if (e.button === 0) isFiring = true; });
  window.addEventListener('mouseup', e => { if (e.button === 0) isFiring = false; });

  document.getElementById('start-btn').addEventListener('click', startGame);
  document.getElementById('restart-btn').addEventListener('click', startGame);

  UIManager.showStartScreen();
  animate();
}

function startGame() {
  GameEngine.score = 0;
  GameEngine.timeScale = 1.0;
  GameEngine.state = 'playing';
  PlayerController.reset();
  ZombieManager.reset();
  WeaponSystem.reset();
  SkillSystem.reset();
  ParticleSystem.reset();
  WaveManager.reset();
  UIManager.hideStartScreen();
  document.getElementById('gameover-screen').style.display = 'none';
  lastWave = 0;
  WaveManager.startNextWave();
  UIManager.announceWave(1);
}

function gameOver() {
  GameEngine.state = 'gameover';
  UIManager.showGameOver();
}

function animate() {
  requestAnimationFrame(animate);
  GameEngine.updateDelta();

  if (GameEngine.state === 'playing') {
    PlayerController.updateMouseWorld(GameEngine.camera, mouseNDC);
    PlayerController.update(GameEngine.scaledDelta);
    PhysicsWorld.checkPlayerCollision(PlayerController.pos, PlayerController.radius);
    PlayerController.mesh.position.copy(PlayerController.pos);

    WeaponSystem.fire(PlayerController.pos, PlayerController.aimAngle, isFiring);
    WeaponSystem.update(GameEngine.scaledDelta);

    ZombieManager.update(GameEngine.scaledDelta, PlayerController.pos);
    PhysicsWorld.checkZombieCollisions(ZombieManager.zombies);

    SkillSystem.update(GameEngine.delta, GameEngine.scaledDelta);
    ParticleSystem.update(GameEngine.scaledDelta);
    WaveManager.update(GameEngine.scaledDelta);

    if (WaveManager.currentWave !== lastWave) {
      lastWave = WaveManager.currentWave;
      if (lastWave > 0) UIManager.announceWave(lastWave);
    }

    GameEngine.updateCamera(PlayerController.pos.x, PlayerController.pos.z);
    UIManager.update();

    if (PlayerController.health <= 0) gameOver();
  }

  GameEngine.render();
}

initGame();