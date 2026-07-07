class Player {
  constructor() {
    this.health = 100;
    this.maxHealth = 100;
    this.speed = 12;
    this.radius = 0.6;
    this.angle = 0;
    const group = new THREE.Group();
    const body = new THREE.Mesh(new THREE.CylinderGeometry(0.4, 0.35, 1.2, 8), new THREE.MeshStandardMaterial({ color: 0x4488ff, roughness: 0.6 }));
    body.position.y = 0.8; body.castShadow = true; group.add(body);
    const head = new THREE.Mesh(new THREE.SphereGeometry(0.3, 12, 12), new THREE.MeshStandardMaterial({ color: 0xffcc88, roughness: 0.5 }));
    head.position.y = 1.6; head.castShadow = true; group.add(head);
    const gun = new THREE.Mesh(new THREE.BoxGeometry(0.15, 0.15, 0.6), new THREE.MeshStandardMaterial({ color: 0x333333 }));
    gun.position.set(0.3, 1.0, 0.4); group.add(gun);
    this.mesh = group;
    Game.scene.add(this.mesh);
  }
  update(dt) {
    const move = new THREE.Vector3();
    if (Game.keys['KeyW']) move.z -= 1;
    if (Game.keys['KeyS']) move.z += 1;
    if (Game.keys['KeyA']) move.x -= 1;
    if (Game.keys['KeyD']) move.x += 1;
    if (move.lengthSq() > 0) { move.normalize().multiplyScalar(this.speed * dt); this.mesh.position.add(move); }
    const lim = Game.WORLD_SIZE - 1;
    this.mesh.position.x = Math.max(-lim, Math.min(lim, this.mesh.position.x));
    this.mesh.position.z = Math.max(-lim, Math.min(lim, this.mesh.position.z));
    const dx = Game.mouse.worldX - this.mesh.position.x;
    const dz = Game.mouse.worldZ - this.mesh.position.z;
    this.angle = Math.atan2(dx, dz);
    this.mesh.rotation.y = this.angle;
  }
  getMuzzlePos() {
    const offset = new THREE.Vector3(0.3, 1.0, 0.7);
    offset.applyAxisAngle(new THREE.Vector3(0, 1, 0), this.angle);
    offset.add(this.mesh.position);
    return offset;
  }
  getAimDir() {
    const dx = Game.mouse.worldX - this.mesh.position.x;
    const dz = Game.mouse.worldZ - this.mesh.position.z;
    return new THREE.Vector3(dx, 0, dz).normalize();
  }
  takeDamage(amount) {
    this.health -= amount;
    if (this.health <= 0) { this.health = 0; Game.gameOver(); }
  }
}