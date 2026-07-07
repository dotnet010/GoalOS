import * as THREE from 'three';

export function createAquarium(width = 20, height = 12, depth = 14) {
  const group = new THREE.Group();

  // 透明玻璃容器
  const glassGeo = new THREE.BoxGeometry(width, height, depth);
  const glassMat = new THREE.MeshPhysicalMaterial({
    color: 0xaaccff,
    metalness: 0,
    roughness: 0.05,
    transmission: 0.95,
    transparent: true,
    opacity: 0.15,
    side: THREE.DoubleSide,
    ior: 1.33,
    depthWrite: false
  });
  const glass = new THREE.Mesh(glassGeo, glassMat);
  group.add(glass);

  // 玻璃边框线
  const edges = new THREE.EdgesGeometry(glassGeo);
  const frameMat = new THREE.LineBasicMaterial({ color: 0x4499cc });
  const frame = new THREE.LineSegments(edges, frameMat);
  group.add(frame);

  // 底座
  const baseGeo = new THREE.BoxGeometry(width + 1, 0.5, depth + 1);
  const baseMat = new THREE.MeshStandardMaterial({ color: 0x1a1a2e, roughness: 0.8 });
  const base = new THREE.Mesh(baseGeo, baseMat);
  base.position.y = -height / 2 - 0.25;
  group.add(base);

  // 沙底
  const sandGeo = new THREE.BoxGeometry(width - 0.4, 0.3, depth - 0.4);
  const sandMat = new THREE.MeshStandardMaterial({ color: 0xc2b280, roughness: 1 });
  const sand = new THREE.Mesh(sandGeo, sandMat);
  sand.position.y = -height / 2 + 0.15;
  group.add(sand);

  // 水体体积（半透明蓝色）
  const waterGeo = new THREE.BoxGeometry(width - 0.2, height - 0.4, depth - 0.2);
  const waterMat = new THREE.MeshPhongMaterial({
    color: 0x006994,
    transparent: true,
    opacity: 0.12,
    side: THREE.DoubleSide
  });
  const water = new THREE.Mesh(waterGeo, waterMat);
  group.add(water);

  // 鱼缸碰撞边界（留出余量防止鱼穿墙）
  const bounds = {
    minX: -width / 2 + 1,
    maxX: width / 2 - 1,
    minY: -height / 2 + 1,
    maxY: height / 2 - 1,
    minZ: -depth / 2 + 1,
    maxZ: depth / 2 - 1
  };

  group.userData.bounds = bounds;
  group.userData.glass = glass;

  return group;
}