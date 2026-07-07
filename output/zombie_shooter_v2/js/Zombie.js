// ZombieManager - 3 zombie types, AI, spawning, death
const ZombieManager = {
  zombies: [],
  scene: null,

  init(scene) {
    this.scene = scene;
    this.zombies = [];
  },

  spawn(type, x, z) {
    const mesh = AssetFactory.createZombie(type);
    mesh.position.set(x, 0, z);
    this.scene.add(mesh);
    this.zombies.push({
      mesh, pos: new THREE.Vector3(x, 0, z),
      hp: mesh.userData.hp, maxHp: mesh.userData.maxHp,
      speed: mesh.userData.speed, dmg: mesh.userData.dmg,
      radius: mesh.userData.radius, type,
      attackCD: 0, hitFlash: 0, walkPhase: Math.random() * Math.PI * 2
    });
  },

  spawnAtEdge(type) {
    const a = Math.random() * Math.PI * 2;
    this.spawn(type, Math.cos(a) * 45, Math.sin(a) * 45);
  },

  update(delta, playerPos) {
    for (let i = this.zombies.length - 1; i >= 0; i--) {
      const z = this.zombies[i];
      const dx = playerPos.x - z.pos.x;
      const dz = playerPos.z - z.pos.z;
      const dist = Math.sqrt(dx*dx + dz*dz);
      if (dist > 0.01) {
        z.pos.x += (dx / dist) * z.speed * delta;
        z.pos.z += (dz / dist) * z.speed * delta;
      }
      z.mesh.rotation.y = Math.atan2(dx, dz);

      z.walkPhase += delta * z.speed * 3;
      const sw = Math.sin(z.walkPhase) * 0.3;
      if (z.mesh.children[4]) z.mesh.children[4].rotation.x = -Math.PI/3 + sw;
      if (z.mesh.children[5]) z.mesh.children[5].rotation.x = -Math.PI/3 - sw;

      z.attackCD -= delta;
      if (dist < z.radius + 0.8 && z.attackCD <= 0) {
        z.attackCD = 1.0;
        PlayerController.takeDamage(z.dmg);
      }

      if (z.hitFlash > 0) {
        z.hitFlash -= delta;
        const flash = z.hitFlash > 0;
        z.mesh.children.forEach(c => {
          if (c.material && c.material.emissive) {
            c.material.emissive.setHex(flash ? 0xff0000 : 0x000000);
          }
        });
      }

      z.mesh.position.copy(z.pos);

      if (z.hp <= 0) {
        this.scene.remove(z.mesh);
        this.zombies.splice(i, 1);
        const scores = { normal: 100, fast: 150, large: 300 };
        GameEngine.score += scores[z.type] || 100;
        ParticleSystem.spawnBlood(z.pos);
      }
    }
  },

  hitZombie(zombie, damage) {
    zombie.hp -= damage;
    zombie.hitFlash = 0.15;
  },

  getZombiesInRange(pos, range) {
    return this.zombies.filter(z => z.pos.distanceTo(pos) <= range);
  },

  clear() {
    this.zombies.forEach(z => this.scene.remove(z.mesh));
    this.zombies = [];
  },

  reset() { this.clear(); }
};