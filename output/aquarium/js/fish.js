import * as THREE from 'three';

export class Clownfish {
  constructor(scene, bounds, color = 0xff6600) {
    this.bounds = bounds;
    this.originalColor = color;
    this.scene = scene;
    this.group = new THREE.Group();

    // 鱼身（拉长椭球）
    const bodyGeo = new THREE.SphereGeometry(0.6, 16, 12);
    bodyGeo.scale(1.6, 0.9, 0.5);
    this.bodyMat = new THREE.MeshStandardMaterial({
      color: color,
      roughness: 0.4,
      emissive: 0x000000
    });
    this.body = new THREE.Mesh(bodyGeo, this.bodyMat);
    this.group.add(this.body);

    // 白色条纹1
    const stripe1Geo = new THREE.SphereGeometry(0.62, 16, 12);
    stripe1Geo.scale(0.15, 0.9, 0.5);
    const stripeMat = new THREE.MeshStandardMaterial({ color: 0xffffff, roughness: 0.4 });
    const stripe1 = new THREE.Mesh(stripe1Geo, stripeMat);
    stripe1.position.x = 0.3;
    this.group.add(stripe1);

    // 白色条纹2
    const stripe2Geo = new THREE.SphereGeometry(0.62, 16, 12);
    stripe2Geo.scale(0.12, 0.9, 0.5);
    const stripe2 = new THREE.Mesh(stripe2Geo, stripeMat);
    stripe2.position.x = -0.3;
    this.group.add(stripe2);

    // 尾鳍
    const tailGeo = new THREE.ConeGeometry(0.5, 0.8, 4);
    tailGeo.rotateZ(Math.PI / 2);
    const finMat = new THREE.MeshStandardMaterial({
      color: color,
      roughness: 0.5,
      side: THREE.DoubleSide,
      transparent: true,
      opacity: 0.85
    });
    this.tail = new THREE.Mesh(tailGeo, finMat);
    this.tail.position.x = -1.1;
    this.tail.scale.set(0.6, 1, 0.3);
    this.group.add(this.tail);

    // 背鳍
    const dorsalGeo = new THREE.ConeGeometry(0.3, 0.6, 4);
    const dorsal = new THREE.Mesh(dorsalGeo, finMat);
    dorsal.position.set(0, 0.5, 0);
    dorsal.scale.set(0.8, 1, 0.5);
    this.group.add(dorsal);

    // 眼睛
    const eyeGeo = new THREE.SphereGeometry(0.12, 8, 8);
    const eyeMat = new THREE.MeshStandardMaterial({ color: 0x000000 });
    const eye1 = new THREE.Mesh(eyeGeo, eyeMat);
    eye1.position.set(0.7, 0.15, 0.25);
    this.group.add(eye1);
    const eye2 = new THREE.Mesh(eyeGeo, eyeMat);
    eye2.position.set(0.7, 0.15, -0.25);
    this.group.add(eye2);

    // 物理状态初始化
    this.position = new THREE.Vector3(
      THREE.MathUtils.lerp(bounds.minX, bounds.maxX, Math.random()),
      THREE.MathUtils.lerp(bounds.minY, bounds.maxY, Math.random()),
      THREE.MathUtils.lerp(bounds.minZ, bounds.maxZ, Math.random())
    );
    this.velocity = new THREE.Vector3(
      (Math.random() - 0.5) * 0.05,
      (Math.random() - 0.5) * 0.03,
      (Math.random() - 0.5) * 0.05
    );
    this.baseSpeed = 0.04;
    this.speed = this.baseSpeed;
    this.targetDirection = this.velocity.clone().normalize();
    this.changeDirectionTimer = 0;
    this.changeDirectionInterval = 60 + Math.random() * 120;
    this.scared = false;
    this.scareTimer = 0;
    this.colorTimer = 0;

    this.group.position.copy(this.position);
    scene.add(this.group);

    // 存储可点击网格用于Raycaster
    this.body.userData.fish = this;
    this.interactiveMeshes = [this.body];
  }

  pickNewDirection() {
    this.targetDirection.set(
      (Math.random() - 0.5) * 2,
      (Math.random() - 0.5) * 1,
      (Math.random() - 0.5) * 2
    ).normalize();
  }

  scare() {
    this.scared = true;
    this.scareTimer = 120;
    this.speed = this.baseSpeed * 3;
    this.bodyMat.color.setHex(0xffeeaa);
    this.bodyMat.emissive.setHex(0x442200);
    this.colorTimer = 120;
    this.pickNewDirection();
  }

  update() {
    // 惊吓计时器
    if (this.scareTimer > 0) {
      this.scareTimer--;
      if (this.scareTimer === 0) {
        this.scared = false;
        this.speed = this.baseSpeed;
      }
    }

    // 颜色恢复
    if (this.colorTimer > 0) {
      this.colorTimer--;
      if (this.colorTimer === 0) {
        this.bodyMat.color.setHex(this.originalColor);
        this.bodyMat.emissive.setHex(0x000000);
      }
    }

    // 随机变向
    this.changeDirectionTimer++;
    if (this.changeDirectionTimer >= this.changeDirectionInterval) {
      this.pickNewDirection();
      this.changeDirectionTimer = 0;
      this.changeDirectionInterval = 60 + Math.random() * 120;
    }

    // 平滑插值速度朝目标方向
    const desired = this.targetDirection.clone().multiplyScalar(this.speed);
    this.velocity.lerp(desired, 0.05);

    // 应用速度
    this.position.add(this.velocity);

    // 边界碰撞——物理反弹（不可穿过缸壁）
    if (this.position.x < this.bounds.minX) {
      this.position.x = this.bounds.minX;
      this.targetDirection.x = Math.abs(this.targetDirection.x);
      this.velocity.x = Math.abs(this.velocity.x);
    }
    if (this.position.x > this.bounds.maxX) {
      this.position.x = this.bounds.maxX;
      this.targetDirection.x = -Math.abs(this.targetDirection.x);
      this.velocity.x = -Math.abs(this.velocity.x);
    }
    if (this.position.y < this.bounds.minY) {
      this.position.y = this.bounds.minY;
      this.targetDirection.y = Math.abs(this.targetDirection.y);
      this.velocity.y = Math.abs(this.velocity.y);
    }
    if (this.position.y > this.bounds.maxY) {
      this.position.y = this.bounds.maxY;
      this.targetDirection.y = -Math.abs(this.targetDirection.y);
      this.velocity.y = -Math.abs(this.velocity.y);
    }
    if (this.position.z < this.bounds.minZ) {
      this.position.z = this.bounds.minZ;
      this.targetDirection.z = Math.abs(this.targetDirection.z);
      this.velocity.z = Math.abs(this.velocity.z);
    }
    if (this.position.z > this.bounds.maxZ) {
      this.position.z = this.bounds.maxZ;
      this.targetDirection.z = -Math.abs(this.targetDirection.z);
      this.velocity.z = -Math.abs(this.velocity.z);
    }

    // 更新位置
    this.group.position.copy(this.position);

    // 朝运动方向旋转（鱼头朝+X，lookAt使-Z朝目标，需补偿旋转90度）
    if (this.velocity.length() > 0.001) {
      const lookTarget = this.position.clone().add(this.velocity);
      this.group.lookAt(lookTarget);
      this.group.rotateY(Math.PI / 2);
    }

    // 尾鳍摆动（受惊时加速摆动）
    const wiggleSpeed = this.scared ? 0.03 : 0.01;
    const wiggleAmp = this.scared ? 0.3 : 0.15;
    this.tail.rotation.z = Math.sin(Date.now() * wiggleSpeed) * wiggleAmp;
  }
}